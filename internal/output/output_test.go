package output

import (
	"bytes"
	"testing"

	"github.com/musher-dev/mush/internal/terminal"
	"github.com/musher-dev/mush/internal/testutil"
)

// testTerminal returns a terminal.Info for testing (non-TTY, no color).
func testTerminal() *terminal.Info {
	return &terminal.Info{
		IsTTY:   false,
		NoColor: true,
		Width:   80,
		Height:  24,
	}
}

func TestWriter_Print(t *testing.T) {
	tests := []struct {
		name   string
		quiet  bool
		format string
		args   []interface{}
		want   string
	}{
		{
			name:   "normal output",
			quiet:  false,
			format: "Hello, %s!",
			args:   []interface{}{"world"},
			want:   "Hello, world!",
		},
		{
			name:   "quiet mode suppresses output",
			quiet:  true,
			format: "Hello, %s!",
			args:   []interface{}{"world"},
			want:   "",
		},
		{
			name:   "no args",
			quiet:  false,
			format: "Simple message",
			args:   nil,
			want:   "Simple message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			w := NewWriter(&buf, &buf, testTerminal())
			w.Quiet = tt.quiet

			w.Print(tt.format, tt.args...)

			if got := buf.String(); got != tt.want {
				t.Errorf("Print() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriter_Println(t *testing.T) {
	tests := []struct {
		name  string
		quiet bool
		args  []interface{}
		want  string
	}{
		{
			name:  "normal output",
			quiet: false,
			args:  []interface{}{"Hello", "world"},
			want:  "Hello world\n",
		},
		{
			name:  "quiet mode suppresses output",
			quiet: true,
			args:  []interface{}{"Hello"},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			w := NewWriter(&buf, &buf, testTerminal())
			w.Quiet = tt.quiet

			w.Println(tt.args...)

			if got := buf.String(); got != tt.want {
				t.Errorf("Println() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriter_Error(t *testing.T) {
	var outBuf, errBuf bytes.Buffer

	w := NewWriter(&outBuf, &errBuf, testTerminal())

	w.Error("Error: %s", "something went wrong")

	want := "Error: something went wrong"
	if got := errBuf.String(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	if outBuf.Len() > 0 {
		t.Errorf("Error() should not write to stdout, got %q", outBuf.String())
	}
}

func TestWriter_Errorln(t *testing.T) {
	var outBuf, errBuf bytes.Buffer

	w := NewWriter(&outBuf, &errBuf, testTerminal())

	w.Errorln("Error occurred")

	want := "Error occurred\n"
	if got := errBuf.String(); got != want {
		t.Errorf("Errorln() = %q, want %q", got, want)
	}
}

func TestWriter_PrintJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
		want    string
	}{
		{
			name:    "simple map",
			data:    map[string]string{"key": "value"},
			wantErr: false,
			want:    "{\n  \"key\": \"value\"\n}\n",
		},
		{
			name:    "struct",
			data:    struct{ Name string }{"test"},
			wantErr: false,
			want:    "{\n  \"Name\": \"test\"\n}\n",
		},
		{
			name:    "nil",
			data:    nil,
			wantErr: false,
			want:    "null\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			w := NewWriter(&buf, &buf, testTerminal())

			err := w.PrintJSON(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("PrintJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got := buf.String(); got != tt.want {
				t.Errorf("PrintJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriter_Write(t *testing.T) {
	tests := []struct {
		name  string
		quiet bool
		input []byte
		wantN int
		want  string
	}{
		{
			name:  "normal write",
			quiet: false,
			input: []byte("test data"),
			wantN: 9,
			want:  "test data",
		},
		{
			name:  "quiet mode returns length but no output",
			quiet: true,
			input: []byte("test data"),
			wantN: 9,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			w := NewWriter(&buf, &buf, testTerminal())
			w.Quiet = tt.quiet

			n, err := w.Write(tt.input)
			if err != nil {
				t.Errorf("Write() error = %v", err)
				return
			}

			if n != tt.wantN {
				t.Errorf("Write() n = %d, want %d", n, tt.wantN)
			}

			if got := buf.String(); got != tt.want {
				t.Errorf("Write() output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefault(t *testing.T) {
	w := Default()

	if w.Out == nil {
		t.Error("Default().Out should not be nil")
	}

	if w.Err == nil {
		t.Error("Default().Err should not be nil")
	}

	if w.JSON {
		t.Error("Default().JSON should be false")
	}

	if w.Quiet {
		t.Error("Default().Quiet should be false")
	}

	if w.terminal == nil {
		t.Error("Default().terminal should not be nil")
	}
}

func TestWriter_Debug(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())
	w.Debug("debug message %s", "test")

	if buf.Len() != 0 {
		t.Fatalf("Debug() should not write to output buffers, got %q", buf.String())
	}
}

func TestWriter_Success(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	w.Success("Operation completed")

	// Should contain checkmark and message
	got := buf.String()
	if got == "" {
		t.Error("Success() should produce output")
	}

	if !containsString(got, CheckMark) {
		t.Errorf("Success() should contain checkmark, got %q", got)
	}

	if !containsString(got, "Operation completed") {
		t.Errorf("Success() should contain message, got %q", got)
	}
}

func TestWriter_Failure(t *testing.T) {
	var outBuf, errBuf bytes.Buffer

	w := NewWriter(&outBuf, &errBuf, testTerminal())

	w.Failure("Operation failed")

	// Should write to stderr with X mark
	got := errBuf.String()
	if got == "" {
		t.Error("Failure() should produce output")
	}

	if !containsString(got, XMark) {
		t.Errorf("Failure() should contain X mark, got %q", got)
	}

	if !containsString(got, "Operation failed") {
		t.Errorf("Failure() should contain message, got %q", got)
	}
}

func TestWriter_Warning(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	w.Warning("Be careful")

	got := buf.String()
	if got == "" {
		t.Error("Warning() should produce output")
	}

	if !containsString(got, WarningMark) {
		t.Errorf("Warning() should contain warning mark, got %q", got)
	}
}

func TestWriter_Info(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	w.Info("Information")

	got := buf.String()
	if got == "" {
		t.Error("Info() should produce output")
	}

	if !containsString(got, InfoMark) {
		t.Errorf("Info() should contain info mark, got %q", got)
	}
}

func TestWriter_Muted(t *testing.T) {
	tests := []struct {
		name    string
		quiet   bool
		wantOut bool
	}{
		{
			name:    "normal mode shows muted",
			quiet:   false,
			wantOut: true,
		},
		{
			name:    "quiet mode hides muted",
			quiet:   true,
			wantOut: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			w := NewWriter(&buf, &buf, testTerminal())
			w.Quiet = tt.quiet

			w.Muted("muted text")

			hasOutput := buf.Len() > 0
			if hasOutput != tt.wantOut {
				t.Errorf("Muted() hasOutput = %v, want %v", hasOutput, tt.wantOut)
			}
		})
	}
}

func TestWriter_Context(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	ctx := w.WithContext(t.Context())
	retrieved := FromContext(ctx)

	if retrieved != w {
		t.Error("FromContext should return the same writer")
	}
}

func TestFromContext_Default(t *testing.T) {
	// When no writer in context, should return default
	w := FromContext(t.Context())

	if w == nil {
		t.Error("FromContext should return non-nil writer")
	}
}

func TestWriter_Terminal(t *testing.T) {
	term := testTerminal()

	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, term)

	if w.Terminal() != term {
		t.Error("Terminal() should return the terminal info")
	}
}

func TestWriter_SetNoColor(t *testing.T) {
	var buf bytes.Buffer

	term := &terminal.Info{IsTTY: true, NoColor: false}
	w := NewWriter(&buf, &buf, term)

	w.SetNoColor(true)

	if !term.ForceFlag {
		t.Error("SetNoColor(true) should set ForceFlag")
	}
}

func TestSpinner_Disabled(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())
	w.Quiet = true

	s := w.Spinner("Loading")

	if !s.disabled {
		t.Error("Spinner should be disabled in quiet mode")
	}

	// Should not panic
	s.Start()
	s.UpdateMessage("Updated")
	s.Stop()
}

func TestSpinner_StopWithSuccess(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	s := w.Spinner("Loading")
	s.Start()
	s.StopWithSuccess("Done")

	// Should contain output
	if buf.Len() == 0 {
		t.Error("StopWithSuccess should produce output")
	}
}

func TestSpinner_StopWithFailure(t *testing.T) {
	var outBuf, errBuf bytes.Buffer

	w := NewWriter(&outBuf, &errBuf, testTerminal())

	s := w.Spinner("Loading")
	s.Start()
	s.StopWithFailure("Failed")

	// Should contain some output
	totalOutput := outBuf.Len() + errBuf.Len()
	if totalOutput == 0 {
		t.Error("StopWithFailure should produce output")
	}
}

func TestSpinner_StopWithWarning(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	s := w.Spinner("Loading")
	s.Start()
	s.StopWithWarning("Warning")

	if buf.Len() == 0 {
		t.Error("StopWithWarning should produce output")
	}
}

func TestStatusSymbols(t *testing.T) {
	// Verify symbols are defined
	if CheckMark == "" {
		t.Error("CheckMark should not be empty")
	}

	if XMark == "" {
		t.Error("XMark should not be empty")
	}

	if WarningMark == "" {
		t.Error("WarningMark should not be empty")
	}

	if InfoMark == "" {
		t.Error("InfoMark should not be empty")
	}
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

// Golden file tests for output format stability

func TestPrintJSON_Golden(t *testing.T) {
	type TestStruct struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Active  bool   `json:"active"`
	}

	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	err := w.PrintJSON(TestStruct{
		Name:    "mush",
		Version: "1.0.0",
		Active:  true,
	})
	if err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}

	testutil.AssertGolden(t, buf.String(), "json_output.golden")
}

func TestStatusMessages_Golden(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf, &buf, testTerminal())

	w.Success("Operation completed successfully")
	w.Warning("This is a warning message")
	w.Info("Information for the user")
	w.Muted("Subtle context information")

	testutil.AssertGolden(t, buf.String(), "status_messages.golden")
}
