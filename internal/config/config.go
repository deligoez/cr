// Package config resolves cr's effective configuration.
//
// The layers and their order are fixed by spec/0.1.0.md §2.7: command-line
// flags, then CR_-prefixed environment variables, then the per-repository
// config, then the global config, then the built-in defaults. Resolution
// happens at read time, so no layer is ever baked into a file.
//
// Five decisions are deliberately outside that surface: the confirmation gate
// of §8.5, the argued forcing of §6.3, the question label of §8.1.4, and the
// provenance and evidence regions of §8.1.6 and §8.1.7. The last three are the
// only channels carrying the register, the disclosure, and the checkability to
// the author, so a name that would address any of them is rejected here rather
// than by a caller, and no later code path is in a position to honour it.
//
// One setting has a closed domain rather than a free-form value: §8.1.1's
// render.lang, whose enumeration and built-in question labels live in
// internal/render. It is checked once every layer has settled, so a language cr
// has no label for is refused at resolution and not at the post that needed it.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// EnvPrefix marks an environment variable as addressing a setting (§2.7).
const EnvPrefix = "CR_"

// setting is one configurable key together with its built-in default, the
// lowest layer of §2.7. This table is the whole configuration surface: a name
// absent from it is not a setting, and no layer can introduce one.
type setting struct {
	key string
	def any
}

// settings holds every key the spec gives a default, and that default.
var settings = []setting{
	{"cluster.gap_lines", 12},
	{"cluster.max_lines", 80},
	{"ignore.globs", []string{}},
	{"intent.cmd", []string{"jira", "issue", "view", "{key}", "--plain"}},
	{"intent.key_pattern", `[A-Z][A-Z0-9]+-[0-9]+`},
	{"post.max_comments", 20},
	// The cap on the probe input §8.1.7's evidence region carries, which
	// round 9's unverifiable-evidence-region names and gives no default.
	// It takes §2.4's `tests.output_tail_bytes` default, the one byte cap
	// the spec does give a probe's text, so a region's two fenced blocks
	// are bounded alike. The key is render's, as render.lang's is.
	{render.MaxProbeInputSetting, 4096},
	{"probe.lock_timeout_seconds", 300},
	{"probe.max_per_round", 10},
	{"profile", ""},
	// §4.3.2's two knobs on the reinvention search. min_similarity is the
	// only fractional setting in the table, and it is fractional because
	// §4.3.2 defines similarity as a ratio: rounding it to an integer here
	// would move the threshold to 0 or 1 and either qualify every symbol in
	// the repository or none.
	{"reinvention.max_candidates", 5},
	{"reinvention.min_similarity", 0.6},
	// §8.1.1's language. Its key and its default both come from the domain
	// that owns them, so this table cannot drift from the enumeration the
	// value is checked against in Resolve.
	{render.Setting, render.LangTR.String()},
	{"rules.dead_after", 20},
	{"rules.harvest_min", 3},
	// §7.3.4's two knobs on the demotion rate, which §7.3.6 reuses for
	// the volume candidacy so both candidacies are drawn over the same
	// sample. The threshold is fractional because §7.3.4 defines the rate
	// as a ratio, for the reason reinvention.min_similarity is; the
	// minimum is a count of raised events.
	{"stats.demote_threshold", 0.6},
	{"stats.min_samples", 8},
	// §3.5.3's window: a human thread whose anchor falls within this many
	// lines of a unit's hunks is attached to the unit.
	{"threads.proximity_lines", 10},
}

// defaults indexes settings by key.
var defaults = func() map[string]any {
	byKey := make(map[string]any, len(settings))
	for _, s := range settings {
		byKey[s.key] = s.def
	}
	return byKey
}()

// protectedTokens maps a token appearing in a name to the decision that name
// would address. The check is on the name rather than on an exact key list: a
// list of exact spellings is defeated by any synonym, and the two costs are not
// comparable. Refusing an unrelated key costs a rename; honouring a protected
// one costs an implicit network write, an assertion the register forbids, or a
// disclosure the author never sees.
var protectedTokens = []struct {
	token   string
	subject string
}{
	{"confirm", "the confirmation gate of §8.5"},
	{"gate", "the confirmation gate of §8.5"},
	{"argued", "the argued forcing of §6.3"},
	{"forcing", "the argued forcing of §6.3"},
	{"force", "the argued forcing of §6.3"},
	{"label", "the question label of §8.1.4"},
	{"provenance", "the provenance region of §8.1.6"},
	{"evidence", "the evidence region of §8.1.7"},
}

// carriers are the words that embed a token's letters without naming its
// decision. The list is deliberately short: a word missing from it is refused,
// which costs a rename, while a word wrongly on it makes a protected decision
// addressable from a file.
var carriers = map[string]string{
	"aggregate":   "gate",
	"delegate":    "gate",
	"propagate":   "gate",
	"mitigate":    "gate",
	"navigate":    "gate",
	"enforcing":   "forcing",
	"reinforcing": "forcing",
	"enforce":     "force",
	"reinforce":   "force",
	"workforce":   "force",
}

// ProtectedError reports a name that would address a decision no layer may
// supply.
type ProtectedError struct {
	// Name is the environment variable or configuration key as it was written.
	Name string
	// Subject is the decision the name would address.
	Subject string
}

func (e *ProtectedError) Error() string {
	return fmt.Sprintf(
		"%q is not a setting: it addresses %s, which no configuration layer may supply; remove it",
		e.Name, e.Subject,
	)
}

// checkProtected rejects a name whose spelling addresses a protected decision.
// name is reported to the user; spelling is the part matched against the
// tokens, so an environment variable is judged without its CR_ prefix.
//
// §2.7 refuses a name that "would address" a decision. That is neither an exact
// key list, which any synonym defeats, nor every occurrence of the letters,
// which would refuse "aggregate" and blame the confirmation gate for it. A name
// is read as its words, and a word carrying a token addresses the decision, so
// post.gate_timeout_seconds and post.labels are refused while rules.aggregate_min
// is not.
func checkProtected(name, spelling string) error {
	for _, word := range words(spelling) {
		for _, protected := range protectedTokens {
			if carriers[word] == protected.token {
				continue
			}
			if strings.Contains(word, protected.token) {
				return &ProtectedError{Name: name, Subject: protected.subject}
			}
		}
	}
	return nil
}

// words splits a name into the words a reader sees in it. A separator or a
// camelCase boundary ends a word, and every word is lower-cased, so
// CR_POST_CONFIRM, post.confirm and postConfirm all read alike.
//
// Flushing on Len() >= 0 would append empty words, which no protected token
// can match and no carrier is keyed by, so the deny-list verdict is identical
// either way; that mutant is left deliberately. The separator counts are not
// merely a capacity hint, though: subtracting one from the other is negative
// for a key nested three deep, and `make` panics rather than allocating less.
func words(name string) []string {
	out := make([]string, 0, strings.Count(name, "_")+strings.Count(name, ".")+1)
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			out = append(out, word.String())
			word.Reset()
		}
	}
	var previous rune
	for _, letter := range name {
		switch {
		case !unicode.IsLetter(letter) && !unicode.IsDigit(letter):
			flush()
		case unicode.IsUpper(letter) && unicode.IsLower(previous):
			flush()
			word.WriteRune(unicode.ToLower(letter))
		default:
			word.WriteRune(unicode.ToLower(letter))
		}
		previous = letter
	}
	flush()
	return out
}

// CheckName rejects a field name, in dotted spelling, that addresses a
// protected decision. §6.3.3 names a profile field alongside a setting and an
// environment variable, so a file that is not a configuration layer is judged
// by the same reading of its names as one that is.
func CheckName(name string) error {
	return checkProtected(name, name)
}

// CheckEnviron rejects the first CR_-prefixed variable, in sorted order, whose
// name addresses a protected decision. It reads names only, so it is the check
// every command can make before doing any work, whether or not that command
// resolves the configuration at all.
func CheckEnviron(environ []string) error {
	names := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		// state.HomeEnv carries the state root, not a setting. It is
		// CR_-prefixed but addresses no configuration key, so it passes
		// through untouched.
		if !ok || !strings.HasPrefix(name, EnvPrefix) || name == state.HomeEnv {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := checkProtected(name, strings.TrimPrefix(name, EnvPrefix)); err != nil {
			return err
		}
	}
	return nil
}

// Sources are the four layers a resolution reads above the built-in defaults.
type Sources struct {
	// Flags holds the settings a command's own flags supplied, keyed by
	// setting key. A command adds an entry only for a flag the user actually
	// gave: an unset flag carries its own default, which would mask every
	// layer below it. Flag names are not scanned, because §8.5.2 requires
	// --confirm to exist as a flag. That exemption reaches no protected
	// decision: an entry is keyed by setting key, and the table above is the
	// only surface a flag can reach.
	Flags map[string]any
	// Environ is the environment, in os.Environ form.
	Environ []string
	// RepoConfig is the per-repository config path, empty when no repository
	// is in scope.
	RepoConfig string
	// GlobalConfig is the global config path.
	GlobalConfig string
}

// The five layers of §2.7, named as cr names them to a reader.
//
// They are constants because §2.7's own ordering is the whole of what a reader
// is being told, and a layer spelled one way by the resolution and another way
// by whatever prints it would make two answers out of one fact.
const (
	// LayerFlag is §2.7's first layer, the command line.
	LayerFlag = "command line"
	// LayerEnv is its second, the CR_-prefixed environment.
	LayerEnv = "environment"
	// LayerRepoConfig is its third, the per-repository config file.
	LayerRepoConfig = "per-repository config"
	// LayerGlobalConfig is its fourth, the global config file.
	LayerGlobalConfig = "global config"
	// LayerDefault is its fifth and lowest, the built-in defaults.
	LayerDefault = "built-in default"
)

// Origin is where one setting's value came from: §2.7's layer, and the place a
// reader opens or unsets to change it.
//
// Both are carried because the layer alone does not tell anyone what to do.
// "per-repository config" is a rank; `~/.cr/repos/acme/web/config.json` is the
// file, and which repository's file it was is exactly what a reader who is
// surprised by a value needs. The same holds for the environment, where the
// variable's name is the thing to unset and the layer's name is not.
type Origin struct {
	// From is the layer, one of the five constants above. It is not
	// called Layer, and that is internal/role's fence rather than a
	// preference: §2.5.5's corpus order is resolution layer first, and
	// TestNothingOutsideThisPackageCanNameTheLayerARoleResolvedFrom keeps
	// the bare identifier `Layer` inside internal/role so no other package
	// can order a corpus by it. The reader-facing word is unaffected — the
	// wire field and everything printed still say layer.
	From string `json:"layer"`
	// Source is the config file's path or the environment variable's
	// name. It is empty for the two layers that are not a place a reader
	// can open: the built-in defaults, which are in cr itself, and the
	// command line, which the reader has in front of them.
	Source string `json:"source"`
}

// Config is one resolved configuration: every key of the built-in table
// carrying the value of the highest layer that supplied it, and the layer that
// supplied it.
type Config struct {
	values  map[string]any
	origins map[string]Origin
}

// LayerError reports a layer of §2.7 cr could not use as written: a config file
// it could not read or parse, or a value that is not of its setting's type or
// lies outside its setting's domain.
//
// It carries the layer and the file or variable, and the key when the fault is
// one setting's, because with five layers a refusal naming only the key leaves
// the reader to find which of them said it. §11.2 codes it 3: the command line
// is right, and what cannot be used is the configuration.
type LayerError struct {
	// Origin is the layer, and the file or variable within it.
	Origin Origin
	// Key is the setting whose value was refused, and empty when the file
	// as a whole could not be used.
	Key string
	// Err is the cause.
	Err error
}

func (e *LayerError) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("%s cannot be used: %v", e.place(), e.Err)
	}
	return fmt.Sprintf("%s, as %s sets it, is invalid: %v", e.Key, e.place(), e.Err)
}

func (e *LayerError) Unwrap() error { return e.Err }

// Hint is §12.4's next actionable step, naming the place a reader edits.
func (e *LayerError) Hint() string {
	if e.Key == "" {
		return fmt.Sprintf("repair %s so it parses as a JSON object, or remove it: "+
			"a missing file supplies nothing", e.place())
	}
	return fmt.Sprintf("correct %s in %s to a value the message says it accepts, "+
		"or remove it there so a lower layer supplies it", e.Key, e.place())
}

// place names the layer and where within it the fault sits.
func (e *LayerError) place() string {
	switch e.Origin.From {
	case LayerFlag:
		return "the " + LayerFlag
	case LayerEnv:
		return "the " + LayerEnv + " variable " + e.Origin.Source
	}
	return "the " + e.Origin.From + " file " + e.Origin.Source
}

// Resolve reads every layer and returns the effective configuration.
func Resolve(src Sources) (Config, error) {
	values := make(map[string]any, len(defaults))
	maps.Copy(values, defaults)
	origins := make(map[string]Origin, len(defaults))
	for key := range defaults {
		origins[key] = Origin{From: LayerDefault}
	}
	if err := resolveFiles(values, origins, src); err != nil {
		return Config{}, err
	}
	fromEnvironment, err := fromEnv(src.Environ)
	if err != nil {
		return Config{}, err
	}
	// The variable and not the layer, per §2.7's annotation: `CR_` plus
	// the key is what a reader unsets, and envName is where that spelling
	// already lives.
	fromVariable := func(key string) Origin { return Origin{From: LayerEnv, Source: envName(key)} }
	written, err := apply(values, fromEnvironment, fromVariable)
	if err != nil {
		return Config{}, err
	}
	for _, key := range written {
		origins[key] = fromVariable(key)
	}
	fromFlag := func(string) Origin { return Origin{From: LayerFlag} }
	if written, err = apply(values, src.Flags, fromFlag); err != nil {
		return Config{}, err
	}
	for _, key := range written {
		origins[key] = fromFlag(key)
	}
	resolved := Config{values: values, origins: origins}
	// §8.1.1's language is the one setting with a closed domain, and it is
	// checked here rather than wherever a body is rendered. §8.1.4 builds
	// the question label in per language, so a language cr has no label for
	// leaves §6.3's forcing with nothing to reach the reader through — and
	// the run that would discover it is the run that is about to post. Every
	// layer has settled by this point, so the value checked is the value a
	// command would read, and the layer named is the one that supplied it.
	if _, err := render.ParseLang(resolved.String(render.Setting)); err != nil {
		return Config{}, &LayerError{Origin: origins[render.Setting], Key: render.Setting, Err: err}
	}
	return resolved, nil
}

// resolveFiles applies §2.7's two file layers, lowest first, so the
// per-repository file overwrites the global one.
//
// A path that is empty is a layer that is not in scope — `cr config` names no
// repository unless it is given one — and supplies nothing, exactly as a
// missing file does.
func resolveFiles(values map[string]any, origins map[string]Origin, src Sources) error {
	for _, file := range []struct{ layer, path string }{
		{LayerGlobalConfig, src.GlobalConfig},
		{LayerRepoConfig, src.RepoConfig},
	} {
		if file.path == "" {
			continue
		}
		fromFile := func(string) Origin { return Origin{From: file.layer, Source: file.path} }
		overrides, err := readFile(fromFile(""))
		if err != nil {
			return err
		}
		written, err := apply(values, overrides, fromFile)
		if err != nil {
			return err
		}
		for _, key := range written {
			origins[key] = fromFile(key)
		}
	}
	return nil
}

// apply writes the overrides naming a known setting, coercing each value to the
// type of that setting's default, and returns the keys it wrote. A name that is
// not a setting is ignored: the table is the whole surface, so an unknown key
// configures nothing.
//
// The keys are returned rather than re-derived by the caller because the two
// answers would have to agree about which names this layer settled — an
// unknown key writes nothing and must annotate nothing, and a caller counting
// the overrides instead would credit the layer with a setting it did not
// supply. origin names where a key's value came from, for the refusal of one
// that cannot be coerced.
func apply(values, overrides map[string]any, origin func(key string) Origin) ([]string, error) {
	written := make([]string, 0, len(overrides))
	for _, key := range sortedKeys(overrides) {
		def, known := defaults[key]
		if !known {
			continue
		}
		value, err := coerce(def, overrides[key])
		if err != nil {
			return nil, &LayerError{Origin: origin(key), Key: key, Err: err}
		}
		values[key] = value
		written = append(written, key)
	}
	return written, nil
}

// readFile reads one config.json and returns its dotted keys. A missing file is
// not an error: an absent layer supplies nothing, exactly like an empty one.
func readFile(file Origin) (map[string]any, error) {
	path := file.Source
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, &LayerError{Origin: file, Err: fmt.Errorf("cannot read it: %w", err)}
	}
	var nested map[string]any
	if err := json.Unmarshal(data, &nested); err != nil {
		return nil, &LayerError{Origin: file, Err: fmt.Errorf("it does not parse as a JSON object: %w", err)}
	}
	flat := make(map[string]any, len(nested))
	flatten("", nested, flat)
	for _, key := range sortedKeys(flat) {
		if err := checkProtected(key, key); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return flat, nil
}

// flatten turns a nested config object into the dotted keys of §2.7, so a
// nested object and its dotted spelling address the same setting.
func flatten(prefix string, nested, flat map[string]any) {
	for key, value := range nested {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		if child, ok := value.(map[string]any); ok {
			// An empty object still writes a key, and a key is what the
			// protected scan reads, so it must survive the flattening.
			if len(child) == 0 {
				flat[full] = child
				continue
			}
			flatten(full, child, flat)
			continue
		}
		flat[full] = value
	}
}

// envName is the environment variable addressing key: the CR_ prefix, then the
// key upper-cased with every separator turned into an underscore.
func envName(key string) string {
	return EnvPrefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// fromEnv reads the CR_-prefixed environment. Every CR_-prefixed variable is
// checked against the protected names first, so one is rejected even when it
// addresses no setting cr knows — which is exactly the case §2.7 cares about.
func fromEnv(environ []string) (map[string]any, error) {
	if err := CheckEnviron(environ); err != nil {
		return nil, err
	}
	keyOf := make(map[string]string, len(settings))
	for _, s := range settings {
		keyOf[envName(s.key)] = s.key
	}

	overrides := make(map[string]any, len(environ))
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if key, known := keyOf[name]; known {
			overrides[key] = value
		}
	}
	return overrides, nil
}

// coerce converts a layer's raw value to the type of the setting's default. A
// file supplies JSON types, the environment supplies strings, and a flag
// supplies whatever its own parser produced, so every layer is normalised here
// rather than at each read site.
func coerce(def, raw any) (any, error) {
	switch def.(type) {
	case string:
		return toString(raw)
	case int:
		return toInt(raw)
	case float64:
		return toFloat(raw)
	case []string:
		return toStrings(raw)
	}
	return nil, fmt.Errorf("unsupported setting type %T", def)
}

func toString(raw any) (any, error) {
	if value, ok := raw.(string); ok {
		return value, nil
	}
	return nil, fmt.Errorf("expected a string, got %T", raw)
}

func toInt(raw any) (any, error) {
	switch value := raw.(type) {
	case int:
		return value, nil
	case float64:
		if value != math.Trunc(value) {
			return nil, fmt.Errorf("expected a whole number, got %v", value)
		}
		return int(value), nil
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("expected a whole number, got %q", value)
		}
		return number, nil
	}
	return nil, fmt.Errorf("expected a whole number, got %T", raw)
}

// toFloat reads a fractional setting. A whole number is accepted as itself, so
// a threshold written as `1` in a config file means 1.0 rather than a type
// error: JSON has one number type and a user writing an endpoint of the range
// writes it without a decimal point.
func toFloat(raw any) (any, error) {
	switch value := raw.(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, fmt.Errorf("expected a number, got %q", value)
		}
		return number, nil
	}
	return nil, fmt.Errorf("expected a number, got %T", raw)
}

func toStrings(raw any) (any, error) {
	switch value := raw.(type) {
	case []string:
		return slices.Clone(value), nil
	case []any:
		list := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected a list of strings, got %T in the list", item)
			}
			list = append(list, text)
		}
		return list, nil
	case string:
		var list []string
		if err := json.Unmarshal([]byte(value), &list); err != nil {
			return nil, fmt.Errorf("expected a JSON list of strings, got %q", value)
		}
		if list == nil {
			list = []string{}
		}
		return list, nil
	}
	return nil, fmt.Errorf("expected a list of strings, got %T", raw)
}

// String returns a string setting.
func (c Config) String(key string) string {
	value, _ := c.values[key].(string)
	return value
}

// Int returns an integer setting.
func (c Config) Int(key string) int {
	value, _ := c.values[key].(int)
	return value
}

// Float returns a fractional setting.
func (c Config) Float(key string) float64 {
	value, _ := c.values[key].(float64)
	return value
}

// Strings returns a list setting. The result is a copy and is never nil, so a
// caller can neither alias the resolved value nor serialise it as null.
func (c Config) Strings(key string) []string {
	value, _ := c.values[key].([]string)
	return append(make([]string, 0, len(value)), value...)
}

// Map returns the effective configuration as the flat dotted keys of §2.7,
// ready to print.
func (c Config) Map() map[string]any {
	out := make(map[string]any, len(c.values))
	for key, value := range c.values {
		if list, ok := value.([]string); ok {
			out[key] = append(make([]string, 0, len(list)), list...)
			continue
		}
		out[key] = value
	}
	return out
}

// Origins returns the layer that supplied each setting, keyed as Map is.
//
// Every key of the table is present, because §2.7's lowest layer supplies a
// value for each of them: a setting no file, variable or flag touched came from
// the built-in defaults, and saying so is the annotation. A map missing those
// keys would leave a reader to infer the commonest answer from an absence.
func (c Config) Origins() map[string]Origin {
	out := make(map[string]Origin, len(c.origins))
	maps.Copy(out, c.origins)
	return out
}

// sortedKeys orders a map's keys, so a layer carrying two faults reports the
// same one on every run.
func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
