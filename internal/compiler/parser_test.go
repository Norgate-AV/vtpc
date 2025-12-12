package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Norgate-AV/vtpc/internal/logger"
)

func TestParseVTProOutput_SuccessfulCompilation(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
Main
----------  Successful  ---------
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 0, result.Warnings)
	assert.Equal(t, 0, result.Errors)
	assert.Len(t, result.WarningMessages, 0)
	assert.Len(t, result.ErrorMessages, 0)
}

func TestParseVTProOutput_WithWarnings(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
Main
----------  Successful  ---------
3 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 3, result.Warnings)
	assert.Equal(t, 0, result.Errors)
}

func TestParseVTProOutput_WithErrors(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
Main
----------  Failed  ---------
0 warning(s), 2 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 0, result.Warnings)
	assert.Equal(t, 2, result.Errors)
}

func TestParseVTProOutput_MixedWarningsAndErrors(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
Main
----------  Failed  ---------
5 warning(s), 3 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 5, result.Warnings)
	assert.Equal(t, 3, result.Errors)
}

func TestParseVTProOutput_SingleLineErrorMessage(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: Missing Smart Object reference in control definition.
----------  Failed  ---------
0 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.ErrorMessages, 1)
	assert.Equal(t, "Missing Smart Object reference in control definition.", result.ErrorMessages[0])
}

func TestParseVTProOutput_SingleLineWarningMessage(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ warning ]: Object "Button1" has an unassigned Smart Object ID.
----------  Successful  ---------
1 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Warnings)
	assert.Len(t, result.WarningMessages, 1)
	assert.Equal(t, "Object \"Button1\" has an unassigned Smart Object ID.", result.WarningMessages[0])
}

func TestParseVTProOutput_MultiLineErrorMessage(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: Object "Video Switcher" on Page "Settings" has
	an invalid join number that exceeds the maximum allowed
	value for this device type.
----------  Failed  ---------
0 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.ErrorMessages, 1)
	expected := "Object \"Video Switcher\" on Page \"Settings\" has an invalid join number that " +
		"exceeds the maximum allowed value for this device type."
	assert.Equal(t, expected, result.ErrorMessages[0])
}

func TestParseVTProOutput_MultiLineWarningMessage(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ warning ]: The controls listed are close to exceeding
	the windows path limitations and may cause issues
	during compilation or deployment.
----------  Successful  ---------
1 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Warnings)
	assert.Len(t, result.WarningMessages, 1)
	expected := "The controls listed are close to exceeding the windows path limitations and may " +
		"cause issues during compilation or deployment."
	assert.Equal(t, expected, result.WarningMessages[0])
}

func TestParseVTProOutput_ExactlyFiveContinuationLines(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: This is line one of the error message
	and this is line two
	and this is line three
	and this is line four
	and this is line five
	and this is line six which should be included
----------  Failed  ---------
0 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.ErrorMessages, 1)
	// Should include all 5 continuation lines
	expected := "This is line one of the error message and this is line two and this is line three " +
		"and this is line four and this is line five and this is line six which should be included"
	assert.Equal(t, expected, result.ErrorMessages[0])
}

func TestParseVTProOutput_MoreThanFiveContinuationLines(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: This is line one
	line two
	line three
	line four
	line five
	line six should stop here
	line seven should NOT be included
	line eight should NOT be included
----------  Failed  ---------
0 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.ErrorMessages, 1)
	// Should stop after 5 continuation lines
	expected := "This is line one line two line three line four line five line six should stop here"
	assert.Equal(t, expected, result.ErrorMessages[0])
	// Verify it doesn't contain the lines that should be excluded
	assert.NotContains(t, result.ErrorMessages[0], "line seven")
	assert.NotContains(t, result.ErrorMessages[0], "line eight")
}

func TestParseVTProOutput_MultipleErrors(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: First error message
	[ error ]: Second error message
	[ error ]: Third error message
----------  Failed  ---------
0 warning(s), 3 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 3, result.Errors)
	assert.Len(t, result.ErrorMessages, 3)
	assert.Equal(t, "First error message", result.ErrorMessages[0])
	assert.Equal(t, "Second error message", result.ErrorMessages[1])
	assert.Equal(t, "Third error message", result.ErrorMessages[2])
}

func TestParseVTProOutput_MultipleWarnings(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ warning ]: First warning message
	[ warning ]: Second warning message
----------  Successful  ---------
2 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 2, result.Warnings)
	assert.Len(t, result.WarningMessages, 2)
	assert.Equal(t, "First warning message", result.WarningMessages[0])
	assert.Equal(t, "Second warning message", result.WarningMessages[1])
}

func TestParseVTProOutput_MixedMessages(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ warning ]: First warning
	[ error ]: First error
	[ warning ]: Second warning
	[ error ]: Second error
----------  Failed  ---------
2 warning(s), 2 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 2, result.Warnings)
	assert.Equal(t, 2, result.Errors)
	assert.Len(t, result.WarningMessages, 2)
	assert.Len(t, result.ErrorMessages, 2)
	assert.Equal(t, "First warning", result.WarningMessages[0])
	assert.Equal(t, "Second warning", result.WarningMessages[1])
	assert.Equal(t, "First error", result.ErrorMessages[0])
	assert.Equal(t, "Second error", result.ErrorMessages[1])
}

func TestParseVTProOutput_WithSize(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
Main
----------  Successful  ---------
	[ size ]: 18,588,092 bytes
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, "18,588,092 bytes", result.Size)
}

func TestParseVTProOutput_WithProjectSize(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
Main
----------  Successful  ---------
	[ project size ]: 512 Kb
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, "512 Kb", result.ProjectSize)
}

func TestParseVTProOutput_WithSizeAndProjectSize(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
Main
----------  Successful  ---------
	[ size ]: 18,588,092 bytes
	[ project size ]: 0 Kb
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, "18,588,092 bytes", result.Size)
	assert.Equal(t, "0 Kb", result.ProjectSize)
}

func TestParseVTProOutput_EmptyOutput(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := ``

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 0, result.Warnings)
	assert.Equal(t, 0, result.Errors)
	assert.Len(t, result.WarningMessages, 0)
	assert.Len(t, result.ErrorMessages, 0)
	assert.Equal(t, "", result.Size)
	assert.Equal(t, "", result.ProjectSize)
}

func TestParseVTProOutput_NoSummaryLine(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: Some error message
----------  Failed  ---------`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	// Should still capture error messages even without summary line
	assert.Len(t, result.ErrorMessages, 1)
	assert.Equal(t, "Some error message", result.ErrorMessages[0])
	// But counts will be 0 since no summary line
	assert.Equal(t, 0, result.Errors)
	assert.Equal(t, 0, result.Warnings)
}

func TestParseVTProOutput_WindowsLineEndings(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := "---------- Compiling for TSW-770: [test.vtp] ---------\r\nBoot\r\n" +
		"\t[ error ]: Test error\r\n----------  Failed  ---------\r\n0 warning(s), 1 error(s)"

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.ErrorMessages, 1)
	assert.Equal(t, "Test error", result.ErrorMessages[0])
}

func TestParseVTProOutput_SpecialCharactersInMessages(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: Object "Button's Name" on Page "Home & Away" has invalid characters: @#$%
----------  Failed  ---------
0 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.ErrorMessages, 1)
	assert.Equal(t, "Object \"Button's Name\" on Page \"Home & Away\" has invalid characters: @#$%", result.ErrorMessages[0])
}

func TestParseVTProOutput_EmptyErrorMessage(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]:
----------  Failed  ---------
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	// Empty messages should not be added
	assert.Len(t, result.ErrorMessages, 0)
}

func TestParseVTProOutput_EmptyWarningMessage(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ warning ]:
----------  Successful  ---------
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	// Empty messages should not be added
	assert.Len(t, result.WarningMessages, 0)
}

func TestParseVTProOutput_ContinuationStopsAtMarker(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: First error starts here
	and continues
	[ warning ]: This should start a new warning, not continue the error
----------  Failed  ---------
1 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Errors)
	assert.Equal(t, 1, result.Warnings)
	assert.Len(t, result.ErrorMessages, 1)
	assert.Len(t, result.WarningMessages, 1)
	assert.Equal(t, "First error starts here and continues", result.ErrorMessages[0])
	assert.Equal(t, "This should start a new warning, not continue the error", result.WarningMessages[0])
}

func TestParseVTProOutput_ContinuationStopsAtDashes(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ error ]: Error message starts
	continues here
----------  Failed  ---------
0 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Len(t, result.ErrorMessages, 1)
	// Should stop at the dashes line
	assert.Equal(t, "Error message starts continues here", result.ErrorMessages[0])
	assert.NotContains(t, result.ErrorMessages[0], "Failed")
}

func TestParseVTProOutput_ContinuationStopsAtSummary(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot
	[ warning ]: Warning message starts
	continues here
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Len(t, result.WarningMessages, 1)
	// Should stop at the summary line
	assert.Equal(t, "Warning message starts continues here", result.WarningMessages[0])
	assert.NotContains(t, result.WarningMessages[0], "warning(s)")
}

func TestParseVTProOutput_BlankLinesBetweenMessages(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	output := `---------- Compiling for TSW-770: [test.vtp] ---------
Boot

	[ error ]: First error

	[ error ]: Second error

Main
----------  Failed  ---------
0 warning(s), 2 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 2, result.Errors)
	assert.Len(t, result.ErrorMessages, 2)
	assert.Equal(t, "First error", result.ErrorMessages[0])
	assert.Equal(t, "Second error", result.ErrorMessages[1])
}

func TestParseVTProOutput_RealWorldExample(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	// Real VTPro output format - page names appear as part of compilation progress,
	// but messages themselves use [ warning ]: and [ error ]: markers
	output := `---------- Compiling for TSW-770: [C:\Projects\test.vtp] ---------
Boot
~DummyFlashPage - [ not compiled ]
Main
Settings
	[ warning ]: Object "Video1" on Page "MainPage" has an unassigned Smart Object ID.
	[ error ]: Object "Button1" on Page "Settings" has an invalid join number.
----------  Failed  ---------
	[ size ]: 18,588,092 bytes
	[ project size ]: 0 Kb
1 warning(s), 1 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 1, result.Warnings)
	assert.Equal(t, 1, result.Errors)
	assert.Len(t, result.WarningMessages, 1)
	assert.Len(t, result.ErrorMessages, 1)
	assert.Equal(t, "Object \"Video1\" on Page \"MainPage\" has an unassigned Smart Object ID.", result.WarningMessages[0])
	assert.Equal(t, "Object \"Button1\" on Page \"Settings\" has an invalid join number.", result.ErrorMessages[0])
	assert.Equal(t, "18,588,092 bytes", result.Size)
	assert.Equal(t, "0 Kb", result.ProjectSize)
}

func TestParseVTProOutput_VeryLongPath(t *testing.T) {
	log := logger.NewNoOpLogger()
	c := NewCompiler(log)

	longPath := "C:\\Users\\SomeUser\\Documents\\Projects\\VeryLongProjectName\\SubFolder\\AnotherSubFolder\\test.vtp"
	output := `---------- Compiling for TSW-770: [` + longPath + `] ---------
Boot
Main
----------  Successful  ---------
0 warning(s), 0 error(s)`

	result := &CompileResult{}
	c.parseVTProOutput(output, result)

	assert.Equal(t, 0, result.Warnings)
	assert.Equal(t, 0, result.Errors)
}
