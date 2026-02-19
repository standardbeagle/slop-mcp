package recipes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	recipes := List()
	require.NotEmpty(t, recipes, "should have embedded recipes")

	names := make([]string, len(recipes))
	for i, r := range recipes {
		names[i] = r.Name
		assert.NotEmpty(t, r.Description, "recipe %q should have description", r.Name)
	}

	assert.Contains(t, names, "batch_collect")
	assert.Contains(t, names, "search_and_inspect")
	assert.Contains(t, names, "transform_pipeline")
}

func TestLoad_Exists(t *testing.T) {
	content, err := Load("batch_collect")
	require.NoError(t, err)
	assert.Contains(t, content, "batch_collect")
	assert.NotEmpty(t, content)
}

func TestLoad_NotFound(t *testing.T) {
	_, err := Load("nonexistent_recipe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "batch_collect") // should list available
}

func TestExtractDescription(t *testing.T) {
	desc := extractDescription("batch_collect")
	assert.NotEmpty(t, desc)
	assert.Contains(t, desc, "Run a tool")
}
