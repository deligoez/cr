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
