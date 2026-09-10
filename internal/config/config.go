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
// Two mutants survive here and are left deliberately. The +1 on the capacity
// hint changes only how much is allocated. And flushing on Len() >= 0 would
// append empty words, which no protected token can match, so the deny-list
// verdict is identical either way. Neither is observable; a test written to
// kill them would assert an implementation detail.
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

// Config is one resolved configuration: every key of the built-in table
// carrying the value of the highest layer that supplied it.
type Config struct {
	values map[string]any
}

// Resolve reads every layer and returns the effective configuration.
func Resolve(src Sources) (Config, error) {
	values := make(map[string]any, len(defaults))
	maps.Copy(values, defaults)
	// Files first and lowest layer first, so each later layer overwrites it.
	for _, path := range []string{src.GlobalConfig, src.RepoConfig} {
		if path == "" {
			continue
		}
		overrides, err := readFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := apply(values, overrides, path); err != nil {
			return Config{}, err
		}
	}
	fromEnvironment, err := fromEnv(src.Environ)
	if err != nil {
		return Config{}, err
	}
	if err := apply(values, fromEnvironment, "environment"); err != nil {
		return Config{}, err
	}
	if err := apply(values, src.Flags, "command line"); err != nil {
		return Config{}, err
	}
	resolved := Config{values: values}
	// §8.1.1's language is the one setting with a closed domain, and it is
	// checked here rather than wherever a body is rendered. §8.1.4 builds
	// the question label in per language, so a language cr has no label for
	// leaves §6.3's forcing with nothing to reach the reader through — and
	// the run that would discover it is the run that is about to post. Every
	// layer has settled by this point, so the value checked is the value a
	// command would read.
	if _, err := render.ParseLang(resolved.String(render.Setting)); err != nil {
		return Config{}, err
	}
	return resolved, nil
}

// apply writes the overrides naming a known setting, coercing each value to the
// type of that setting's default. A name that is not a setting is ignored: the
// table is the whole surface, so an unknown key configures nothing.
func apply(values, overrides map[string]any, origin string) error {
	for _, key := range sortedKeys(overrides) {
		def, known := defaults[key]
		if !known {
			continue
		}
		value, err := coerce(def, overrides[key])
		if err != nil {
			return fmt.Errorf("%s: %s is invalid: %w", origin, key, err)
		}
		values[key] = value
	}
	return nil
}

// readFile reads one config.json and returns its dotted keys. A missing file is
// not an error: an absent layer supplies nothing, exactly like an empty one.
func readFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var nested map[string]any
	if err := json.Unmarshal(data, &nested); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
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
	keyOf := make(map[string]string, len(settings))
	for _, s := range settings {
		keyOf[envName(s.key)] = s.key
	}

	names := make([]string, 0, len(environ))
	valueOf := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, EnvPrefix) {
			continue
		}
		// state.HomeEnv carries the state root, not a setting. It is
		// CR_-prefixed but addresses no configuration key, so it passes
		// through untouched.
		if name == state.HomeEnv {
			continue
		}
		names = append(names, name)
		valueOf[name] = value
	}
	slices.Sort(names)

	overrides := make(map[string]any, len(names))
	for _, name := range names {
		if err := checkProtected(name, strings.TrimPrefix(name, EnvPrefix)); err != nil {
			return nil, err
		}
		if key, ok := keyOf[name]; ok {
			overrides[key] = valueOf[name]
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
