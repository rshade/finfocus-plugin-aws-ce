package pricing

import (
	"context"
	"testing"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
)

func TestPluginNameIsTheSingleName(t *testing.T) {
	t.Parallel()

	require.Equal(t, pluginInfoName, PluginName)

	calc := NewCalculator()
	require.Equal(t, PluginName, calc.Name())

	info, err := calc.GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	require.NoError(t, err)
	require.Equal(t, PluginName, info.GetName())
}
