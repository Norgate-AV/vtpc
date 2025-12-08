package vtpro

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSimplWindowsPath_DefaultPath(t *testing.T) {
	// Cannot use t.Parallel() - modifies environment variables

	// Ensure env var is not set
	os.Unsetenv("VTPRO_PATH")

	path := GetVTProPath()
	assert.Equal(t, DefaultVTProPath, path, "Should return default path when env var not set")
}

func TestGetSimplWindowsPath_EnvVarOverride(t *testing.T) {
	// Cannot use t.Parallel() - modifies environment variables

	customPath := "D:\\Custom\\Path\\To\\vtpro.exe"

	// Set env var
	os.Setenv("VTPRO_PATH", customPath)
	defer os.Unsetenv("VTPRO_PATH")

	path := GetVTProPath()
	assert.Equal(t, customPath, path, "Should return env var path when set")
}

func TestGetSimplWindowsPath_EmptyEnvVar(t *testing.T) {
	// Cannot use t.Parallel() - modifies environment variables

	// Set env var to empty string
	os.Setenv("VTPRO_PATH", "")
	defer os.Unsetenv("VTPRO_PATH")

	path := GetVTProPath()
	assert.Equal(t, DefaultVTProPath, path, "Should return default path when env var is empty")
}

func TestValidateSimplWindowsInstallation_DefaultPathNotFound(t *testing.T) {
	// Cannot use t.Parallel() - modifies environment variables

	// Most test environments won't have VTPro installed
	os.Unsetenv("VTPRO_PATH")

	err := ValidateVTProInstallation()
	// On systems without VTPro, we expect an error
	if err != nil {
		assert.Contains(t, err.Error(), "VTPro not found at default path")
		assert.Contains(t, err.Error(), DefaultVTProPath)
	}
	// Note: If VTPro IS installed, err will be nil, which is also valid
}

func TestValidateSimplWindowsInstallation_CustomPathNotFound(t *testing.T) {
	// Cannot use t.Parallel() - modifies environment variables

	nonExistentPath := "Z:\\NonExistent\\Path\\vtpro.exe"

	os.Setenv("VTPRO_PATH", nonExistentPath)
	defer os.Unsetenv("VTPRO_PATH")

	err := ValidateVTProInstallation()

	assert.Error(t, err, "Should return error when custom path does not exist")
	assert.Contains(t, err.Error(), "VTPro not found at custom path")
	assert.Contains(t, err.Error(), nonExistentPath)
	assert.Contains(t, err.Error(), "VTPRO_PATH")
}
