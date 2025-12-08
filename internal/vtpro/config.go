package vtpro

import (
	"fmt"
	"os"
)

const DefaultVTProPath = "C:\\Program Files (x86)\\Crestron\\VtPro-e\\vtpro.exe"

// GetVTProPath returns the path to the VTPro executable.
// It checks the VTPRO_PATH environment variable first,
// falling back to the default installation path if not set.
func GetVTProPath() string {
	if envPath := os.Getenv("VTPRO_PATH"); envPath != "" {
		return envPath
	}

	return DefaultVTProPath
}

// ValidateVTProInstallation checks if the VTPro executable exists.
// Returns an error with helpful guidance if the file is not found.
func ValidateVTProInstallation() error {
	path := GetVTProPath()

	var err error
	if _, err = os.Stat(path); os.IsNotExist(err) {
		if os.Getenv("VTPRO_PATH") != "" {
			return fmt.Errorf("VTPro not found at custom path: %s\n"+
				"Please verify the VTPRO_PATH environment variable is correct", path)
		}

		return fmt.Errorf("VTPro not found at default path: %s\n"+
			"Please install VTPro or set VTPRO_PATH environment variable", path)
	}

	if err != nil {
		return fmt.Errorf("error checking VTPro installation at %s: %w", path, err)
	}

	return nil
}
