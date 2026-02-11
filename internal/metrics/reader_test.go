package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemReader_ReadCPU(t *testing.T) {
	r := NewSystemReader()
	s, err := r.Read(context.Background())
	require.NoError(t, err)

	// CPU total should be 0-100
	assert.GreaterOrEqual(t, s.CPU.TotalPercent, 0.0)
	assert.LessOrEqual(t, s.CPU.TotalPercent, 100.0)

	// Should have at least one core
	assert.NotEmpty(t, s.CPU.PerCore)
	for _, core := range s.CPU.PerCore {
		assert.GreaterOrEqual(t, core, 0.0)
		assert.LessOrEqual(t, core, 100.0)
	}
}

func TestSystemReader_ReadRAM(t *testing.T) {
	r := NewSystemReader()
	s, err := r.Read(context.Background())
	require.NoError(t, err)

	assert.Greater(t, s.RAM.Total, uint64(0))
	assert.LessOrEqual(t, s.RAM.Used, s.RAM.Total)
	assert.GreaterOrEqual(t, s.RAM.Percent, 0.0)
	assert.LessOrEqual(t, s.RAM.Percent, 100.0)
}

func TestSystemReader_ReadDisks(t *testing.T) {
	r := NewSystemReader()
	s, err := r.Read(context.Background())
	require.NoError(t, err)

	// Should have at least one partition
	assert.NotEmpty(t, s.Disks)

	// At least one partition should have a non-zero total (some virtual mounts may report 0)
	var hasReal bool
	for _, d := range s.Disks {
		assert.NotEmpty(t, d.Path)
		assert.GreaterOrEqual(t, d.Percent, 0.0)
		assert.LessOrEqual(t, d.Percent, 100.0)
		if d.Total > 0 {
			hasReal = true
		}
	}
	assert.True(t, hasReal, "expected at least one partition with non-zero total")
}

func TestSystemReader_ReadNetwork(t *testing.T) {
	r := NewSystemReader()
	s, err := r.Read(context.Background())
	require.NoError(t, err)

	// Should have at least one network interface
	assert.NotEmpty(t, s.Networks)

	for _, n := range s.Networks {
		assert.NotEmpty(t, n.Name)
	}

	// First reading: rates should be 0
	for _, n := range s.Networks {
		assert.Equal(t, uint64(0), n.BytesSentPS)
		assert.Equal(t, uint64(0), n.BytesRecvPS)
	}
}

func TestSystemReader_NetworkRateCalculation(t *testing.T) {
	r := NewSystemReader()
	ctx := context.Background()

	// First read — baseline
	_, err := r.Read(ctx)
	require.NoError(t, err)

	// Second read — rates should be calculated (may still be 0 if no traffic)
	s2, err := r.Read(ctx)
	require.NoError(t, err)

	// Just verify it doesn't error and returns interfaces
	assert.NotEmpty(t, s2.Networks)
}

func TestSystemReader_TemperatureNilOrValid(t *testing.T) {
	r := NewSystemReader()
	s, err := r.Read(context.Background())
	require.NoError(t, err)

	// Temperature is either nil (sensor unavailable) or a valid value
	if s.Temperature != nil {
		assert.Greater(t, *s.Temperature, 0.0)
		assert.Less(t, *s.Temperature, 150.0) // sanity check
	}
	// nil is acceptable — not an error
}

func TestSystemReader_SecondReadDoesNotError(t *testing.T) {
	r := NewSystemReader()
	ctx := context.Background()

	// Two consecutive reads should both succeed
	_, err := r.Read(ctx)
	require.NoError(t, err)

	s2, err := r.Read(ctx)
	require.NoError(t, err)
	assert.NotNil(t, s2)
}
