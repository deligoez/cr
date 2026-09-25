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

