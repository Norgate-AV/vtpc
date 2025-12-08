package compiler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Norgate-AV/vtpc/internal/compiler"
)

func TestCompileResult_HasErrors(t *testing.T) {
	tests := []struct {
		name      string
		result    compiler.CompileResult
		hasErrors bool
	}{
		{
			name:      "No errors",
			result:    compiler.CompileResult{Errors: 0, HasErrors: false},
			hasErrors: false,
		},
		{
			name:      "Has errors",
			result:    compiler.CompileResult{Errors: 5, HasErrors: true},
			hasErrors: true,
		},
		{
			name:      "Warnings only",
			result:    compiler.CompileResult{Warnings: 3, Errors: 0, HasErrors: false},
			hasErrors: false,
		},
		{
			name: "Mixed warnings and errors",
			result: compiler.CompileResult{
				Warnings:  3,
				Errors:    2,
				HasErrors: true,
			},
			hasErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.hasErrors, tt.result.HasErrors)
		})
	}
}

func TestCompileResult_MessageCounts(t *testing.T) {
	result := compiler.CompileResult{
		ErrorMessages:   []string{"error1", "error2"},
		WarningMessages: []string{"warn1"},
	}

	assert.Len(t, result.ErrorMessages, 2)
	assert.Len(t, result.WarningMessages, 1)
}

func TestCompileResult_EmptyMessages(t *testing.T) {
	result := compiler.CompileResult{
		Errors:          0,
		Warnings:        0,
		ErrorMessages:   []string{},
		WarningMessages: []string{},
		HasErrors:       false,
	}

	assert.Len(t, result.ErrorMessages, 0)
	assert.Len(t, result.WarningMessages, 0)
	assert.False(t, result.HasErrors)
	assert.Equal(t, 0, result.Errors)
	assert.Equal(t, 0, result.Warnings)
}

func TestCompileResult_CountsMatchMessages(t *testing.T) {
	result := compiler.CompileResult{
		Errors:   2,
		Warnings: 3,
		ErrorMessages: []string{
			"ERROR: Undefined variable X",
			"ERROR: Missing semicolon",
		},
		WarningMessages: []string{
			"WARNING: Unused variable Y",
			"WARNING: Deprecated function",
			"WARNING: Unreachable code",
		},
		HasErrors: true,
	}

	assert.Equal(t, result.Errors, len(result.ErrorMessages))
	assert.Equal(t, result.Warnings, len(result.WarningMessages))
}

func TestCompileOptions_RequiredFields(t *testing.T) {
	opts := compiler.CompileOptions{
		FilePath: "test.vtp",
		Hwnd:     uintptr(12345),
	}

	assert.Equal(t, "test.vtp", opts.FilePath)
	assert.Equal(t, uintptr(12345), opts.Hwnd)
}

func TestCompileOptions_SimplPidPtr(t *testing.T) {
	var pid uint32
	opts := compiler.CompileOptions{
		VTProPidPtr: &pid,
	}

	// Verify pointer is set
	assert.NotNil(t, opts.VTProPidPtr)

	// Verify we can write to it
	*opts.VTProPidPtr = 12345
	assert.Equal(t, uint32(12345), pid)
}
