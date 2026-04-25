package adapter

import "testing"

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "simple",
			in:   "tool --prompt /tmp/prompt.txt",
			want: []string{"tool", "--prompt", "/tmp/prompt.txt"},
		},
		{
			name: "quoted paths",
			in:   `tool --work-dir "/tmp/project with spaces" --prompt '/tmp/prompt file.txt'`,
			want: []string{"tool", "--work-dir", "/tmp/project with spaces", "--prompt", "/tmp/prompt file.txt"},
		},
		{
			name: "escaped spaces",
			in:   `tool --name hello\ world`,
			want: []string{"tool", "--name", "hello world"},
		},
		{
			name: "empty quoted arg",
			in:   `tool --flag ""`,
			want: []string{"tool", "--flag", ""},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := splitCommand(c.in)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("len=%d want=%d got=%#v", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("arg[%d]=%q want %q; all=%#v", i, got[i], c.want[i], got)
				}
			}
		})
	}
}

func TestSplitCommand_UnterminatedQuote(t *testing.T) {
	if _, err := splitCommand(`tool "oops`); err == nil {
		t.Fatal("want error for unterminated quote")
	}
}
