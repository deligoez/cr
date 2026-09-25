package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §2.6.3.8's path shape is the directory and the extension, in the shape a
// rule's `globs` takes.
func TestAPathShapeIsItsDirectoryAndExtension(t *testing.T) {
	for file, shape := range map[string]string{
		"app/Models/User.php":          "app/Models/*.php",
		"tests/Feature/ExportTest.php": "tests/Feature/*.php",
		"composer.json":                "*.json",
		"docker/Makefile":              "docker/*",
		"":                             "",
	} {
		assert.Equal(t, shape, PathShape(file), file)
	}
}

// §2.6.3.8's code spans are the distinct inline spans of a body; a fenced
// block, a suggestion among them, is code the comment proposes and is skipped.
func TestCodeSpansAreTheInlineSpansOutsideFences(t *testing.T) {
	body := "Use `Rule::enum` here, and `Rule::enum` there, not ` `.\n" +
		"```suggestion\n$x = `inside`;\n```\n" +
		"Also `value`."

	assert.Equal(t, []string{"Rule::enum", "value"}, CodeSpans(body))
}

