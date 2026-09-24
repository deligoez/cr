package symbol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TypeScript's four shapes, with the declared parameter count §4.3.2 ranks
// on. A class's count is its constructor's, as PHP's is.
func TestTypescriptDeclarationsAreIndexedWithTheirParameterCounts(t *testing.T) {
	index, built := Build("typescript", []File{fileOf("src/cart.ts", `import { Money } from './money'

export class Cart {
  constructor(private readonly owner: string, items: Item[] = []) {
    this.items = items
  }

  async total(currency: string): Promise<Money> {
    if (this.items.length === 0) {
      return Money.zero(currency)
    }
    return sum(this.items)
  }
}

export interface Item {
  price: number
}

export function sum<T>(items: T[], start = 0): Money {
  return items.reduce((a, b) => a, start)
}

export const discount = (price: number, rate: number): number => price * rate

const grouped = (items.length + 1)
`)})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "src/cart.ts", Line: 3, Name: "Cart", Kind: Class, Params: 2},
		{Path: "src/cart.ts", Line: 4, Name: "constructor", Kind: Method, Params: 2},
		{Path: "src/cart.ts", Line: 8, Name: "total", Kind: Method, Params: 1},
		{Path: "src/cart.ts", Line: 16, Name: "Item", Kind: Class},
		{Path: "src/cart.ts", Line: 20, Name: "sum", Kind: Function, Params: 2},
		{Path: "src/cart.ts", Line: 24, Name: "discount", Kind: Function, Params: 2},
	}, index.Decls, "`if (` is control flow and `(items.length + 1)` is no arrow function")
}

// A Vue single-file component is scanned whole: the template's lines match no
// rule, and the options object's methods are methods. A call handed a
// callback, `watch(source, () => {`, reads like a method up to its paren and
// is not one.
func TestAVueComponentsScriptIsIndexedAndItsTemplateIsNot(t *testing.T) {
	index, built := Build("typescript", []File{fileOf("src/components/Cart.vue", `<template>
  <div v-if="ready" @click="save(item)">{{ total(item) }}</div>
</template>

<script>
export default {
  methods: {
    save(item) {
      store(item)
    },
  },
  mounted() {
    watch(this.source, function (value) {
    })
  },
}
</script>
`)})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "src/components/Cart.vue", Line: 8, Name: "save", Kind: Method, Params: 1},
		{Path: "src/components/Cart.vue", Line: 12, Name: "mounted", Kind: Method},
	}, index.Decls)
}

// Rust's shapes. A method's `self` is its receiver and is not counted, so it
// declares what the same method declares in Go; a lifetime's quote opens no
// string, so the commas after `&'a str` still separate parameters.
func TestRustDeclarationsAreIndexedWithTheirParameterCounts(t *testing.T) {
	index, built := Build("rust", []File{fileOf("src/cart.rs", `pub struct Cart<'a> {
    owner: &'a str,
}

pub(crate) enum Currency {
    Try,
}

impl<'a> Cart<'a> {
    pub fn new(owner: &'a str, currency: Currency) -> Self {
        Cart { owner }
    }

    pub async fn total(&mut self, rate: f64) -> Money {
        let sep = '{';
        Money::zero()
    }

    fn owner(&'a self) -> &'a str {
        self.owner
    }
}

pub fn parse<T: Into<String>>(input: T, strict: bool) -> Option<Cart<'static>> {
    None
}

pub trait Priced {
    fn price(&self) -> u64;
}
`)})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "src/cart.rs", Line: 1, Name: "Cart", Kind: Class},
		{Path: "src/cart.rs", Line: 5, Name: "Currency", Kind: Class},
		{Path: "src/cart.rs", Line: 10, Name: "new", Kind: Method, Params: 2},
		{Path: "src/cart.rs", Line: 14, Name: "total", Kind: Method, Params: 1},
		{Path: "src/cart.rs", Line: 19, Name: "owner", Kind: Method},
		{Path: "src/cart.rs", Line: 24, Name: "parse", Kind: Function, Params: 2},
		{Path: "src/cart.rs", Line: 28, Name: "Priced", Kind: Class},
		{Path: "src/cart.rs", Line: 29, Name: "price", Kind: Method},
	}, index.Decls)
}

// §3.4.4's symbol branch needs a body's end, and a lifetime read as a string
// would move it: `'a` would open a quote the next `'` closes, and every brace
// between the two would count for nothing. A character literal still is one,
// so the `'{'` inside total's body does not open a brace.
func TestARustLifetimeDoesNotMoveTheEndOfABody(t *testing.T) {
	index, built := Build("rust", []File{fileOf("src/cart.rs", `impl<'a> Cart<'a> {
    fn owner(&'a self) -> &str {
        self.owner
    }

    fn brace(&self) -> char {
        '{'
    }
}
`)})
	require.True(t, built)

	owner, found := index.Enclosing("src/cart.rs", 3)
	require.True(t, found)
	assert.Equal(t, "owner@2", owner)

	brace, found := index.Enclosing("src/cart.rs", 7)
	require.True(t, found)
	assert.Equal(t, "brace@6", brace)

	_, found = index.Enclosing("src/cart.rs", 9)
	assert.False(t, found, "the impl block is no declaration, so its closing brace belongs to none")
}

// A lifetime can end its line, as a `where` bound's does, and the quote is
// then the last byte but one: nothing follows the character after it to close
// a literal, so it is a lifetime, and the body after it is still bounded.
func TestALifetimeEndingItsLineIsStillALifetime(t *testing.T) {
	index, built := Build("rust", []File{fileOf("src/keep.rs", `pub fn keep<'a, T>(value: T) -> T
where
    T: 'a
{
    value
}
`)})
	require.True(t, built)

	assert.Equal(t, []Decl{{Path: "src/keep.rs", Line: 1, Name: "keep", Kind: Function, Params: 1}}, index.Decls)
	keep, found := index.Enclosing("src/keep.rs", 5)
	require.True(t, found)
	assert.Equal(t, "keep@1", keep)
}

// enclosing is the name Enclosing gives a line, or "" when none encloses it.
func enclosing(t *testing.T, index *Index, path string, line int) string {
	t.Helper()
	key, found := index.Enclosing(path, line)
	if !found {
		return ""
	}
	return key
}

// `#` opens a private name in TypeScript and a comment only in PHP. Read as a
// comment, `this.#count += by; }` lost its closing brace, the method's body
// ran past the class field below it, and a line in the field's arrow was
// attributed to the method (QA, 2026-09-23).
func TestAPrivateNameDoesNotMoveTheEndOfATypeScriptBody(t *testing.T) {
	index, built := Build("typescript", []File{fileOf("src/c.ts", `class C {
  inc(by: number) {
    if (by) { this.#count += by; }
  }

  handler = () => {
    work();
  };
}
`)})
	require.True(t, built)

	assert.Equal(t, "inc@2", enclosing(t, index, "src/c.ts", 3))
	assert.Equal(t, "C@1", enclosing(t, index, "src/c.ts", 7),
		"the field's arrow is no declaration, so its line is the class's, not inc's")
}

// A single- or double-quoted string cannot span a line in TypeScript, so an
// apostrophe in JSX text closes at the line's end instead of swallowing the
// next function (QA, 2026-09-23: Hello's span was 1–7 and World had none).
func TestAnApostropheInJSXTextDoesNotSwallowTheNextFunction(t *testing.T) {
	index, built := Build("typescript", []File{fileOf("src/hello.tsx", `export function Hello() {
  return <p>Don't do it {name}</p>;
}

export function World() {
  return <p>It's fine</p>;
}
`)})
	require.True(t, built)

	assert.Equal(t, "Hello@1", enclosing(t, index, "src/hello.tsx", 2))
	assert.Equal(t, "World@5", enclosing(t, index, "src/hello.tsx", 6))
}

// A Rust raw string is read verbatim to its closing quote and hashes, so a
// quote or a brace inside it counts for nothing.
func TestARustRawStringDoesNotMoveTheEndOfABody(t *testing.T) {
	index, built := Build("rust", []File{fileOf("src/t.rs", `fn quoted() -> &'static str { r#"a "quoted" { brace"# }

fn after() {
    work();
}
`)})
	require.True(t, built)

	assert.Equal(t, "quoted@1", enclosing(t, index, "src/t.rs", 1))
	assert.Empty(t, enclosing(t, index, "src/t.rs", 2), "quoted's body closed on its own line")
	assert.Equal(t, "after@3", enclosing(t, index, "src/t.rs", 4))
}

// A comma inside a generic type separates no parameter, and a generic bound
// holding an arrow type is read whole. §4.3.2 matches the count exactly, so
// each of these dropped a real candidate (QA, 2026-09-23).
func TestAGenericTypesCommasSeparateNoParameter(t *testing.T) {
	ts, built := Build("typescript", []File{fileOf("src/m.ts", `export function merge(a: Map<string, number>, b: Array<[string, number]>) {
}

export const pick = <T extends Record<string, unknown>>(obj: T, key: string) => obj[key]

export function apply(cb: (x: number) => void, n: number) {
}
`)})
	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "src/m.ts", Line: 1, Name: "merge", Kind: Function, Params: 2},
		{Path: "src/m.ts", Line: 4, Name: "pick", Kind: Function, Params: 2},
		{Path: "src/m.ts", Line: 6, Name: "apply", Kind: Function, Params: 2},
	}, ts.Decls)

	rs, built := Build("rust", []File{fileOf("src/g.rs", `fn group(m: HashMap<String, Vec<u8>>, n: usize) {
}

fn bounded<F: Fn() -> ()>(f: F, g: u8) {
}
`)})
	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "src/g.rs", Line: 1, Name: "group", Kind: Function, Params: 2},
		{Path: "src/g.rs", Line: 4, Name: "bounded", Kind: Function, Params: 2},
	}, rs.Decls)
}

// An arrow function is one whose parameter list is followed by its arrow. A
// parenthesised expression whose line reaches an arrow later is not one, and
// a constructor with an empty body on its own line is still a declaration.
func TestOnlyAParameterListFollowedByItsArrowIsAnArrowFunction(t *testing.T) {
	index, built := Build("typescript", []File{fileOf("src/a.ts", `const toIds = (items ?? []).map((i) => i.id)
const typed = (a: number): number => a + 1

class Cart {
  constructor(private items: Array<[string, number]>) {}
}
`)})
	require.True(t, built)

	assert.Equal(t, []Decl{
		{Path: "src/a.ts", Line: 2, Name: "typed", Kind: Function, Params: 1},
		{Path: "src/a.ts", Line: 4, Name: "Cart", Kind: Class, Params: 1},
		{Path: "src/a.ts", Line: 5, Name: "constructor", Kind: Method, Params: 1},
	}, index.Decls)
}

// Only a single- or double-quoted string ends at its line in TypeScript. A
// template literal runs across lines, so a brace inside one counts for nothing
// however many lines below its backtick it stands.
//
// gremlins found the quote test open: the one multi-line fixture was JSX text,
// so ending every open string at its line, the template literal's too, changed
// nothing any fixture read.
func TestATypeScriptTemplateLiteralRunsAcrossLines(t *testing.T) {
	index, built := Build("typescript", []File{fileOf("src/banner.ts", "export function banner() {\n"+
		"  const text = `\n"+
		"  }\n"+
		"  `;\n"+
		"  return text;\n"+
		"}\n")})
	require.True(t, built)

	assert.Equal(t, "banner@1", enclosing(t, index, "src/banner.ts", 5),
		"the brace inside the template literal closed nothing")
}

// A Rust raw string ends at its own closing quote and hashes, even when that
// closer stands directly after the opening quote, and a content that opens with
// a `#` is content: the `"#` the opening quote forms with it closes nothing.
//
// gremlins found both edges open: the one raw string fixture held content that
// neither was empty nor began with `#`.
func TestARustRawStringEndsAtItsOwnCloserAndNoEarlier(t *testing.T) {
	index, built := Build("rust", []File{fileOf("src/raw.rs", `fn empty() -> &'static str {
    r""
}

fn hashed() -> &'static str {
    r#"#{"#
}

fn after() {
    work();
}
`)})
	require.True(t, built)

	assert.Equal(t, "empty@1", enclosing(t, index, "src/raw.rs", 2))
	assert.Equal(t, "hashed@5", enclosing(t, index, "src/raw.rs", 6))
	assert.Equal(t, "after@9", enclosing(t, index, "src/raw.rs", 10))
}

// A line of a Rust body may begin with `r` at its first byte, and the scan asks
// whether it opens a raw string without reading the byte before the line.
//
// gremlins found the guard open: every fixture's body was indented, so no `r`
// the scan met stood at the start of its line.
func TestAnRAtTheStartOfARustLineIsReadInsideTheLine(t *testing.T) {
	index, built := Build("rust", []File{fileOf("src/early.rs", `fn early() -> u8 {
return 1;
}
`)})
	require.True(t, built)

	assert.Equal(t, "early@1", enclosing(t, index, "src/early.rs", 2))
}

// A Rust byte raw string opens at its `b`, so `br"\"` is a raw string holding a
// backslash and not a byte string whose closing quote the backslash escapes.
//
// gremlins found the `b` step open: no fixture held a byte raw string, and what
// separates the two readings is a backslash before the closing quote.
func TestARustByteRawStringIsRawToo(t *testing.T) {
	index, built := Build("rust", []File{fileOf("src/bytes.rs", `fn bytes() -> &'static [u8] {
    br"\"
}

fn after() {
    work();
}
`)})
	require.True(t, built)

	assert.Equal(t, "bytes@1", enclosing(t, index, "src/bytes.rs", 2))
	assert.Equal(t, "after@5", enclosing(t, index, "src/bytes.rs", 6))
}
