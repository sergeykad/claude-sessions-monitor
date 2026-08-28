package session

import "testing"

// The path is the last column, so a directory name holding a space is part of
// it. Read as the last whitespace-separated field, "/Users/x/My Project" comes
// back as "Project", and the session is then keyed under a project that does
// not exist, so it disappears from the dashboard.
func TestParseLsofCwdKeepsAPathThatHoldsSpaces(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string // empty means the parse must fail
	}{
		{
			name: "an ordinary path",
			out:  "claude  1868 u  cwd  DIR  1,17  832  100 /Users/u/proj\n",
			want: "/Users/u/proj",
		},
		{
			name: "a path holding spaces",
			out:  "claude  1868 u  cwd  DIR  1,17  832  100 /Users/u/My Project\n",
			want: "/Users/u/My Project",
		},
		{
			name: "the cwd line is among others",
			out:  "claude 1868 u txt REG 1,17 100 1 /bin/claude\nclaude 1868 u cwd DIR 1,17 832 100 /Users/u/proj\n",
			want: "/Users/u/proj",
		},
		{
			name: "no cwd line at all",
			out:  "claude 1868 u txt REG 1,17 100 1 /bin/claude\n",
		},
		{
			name: "the cwd line stops before the path",
			out:  "claude 1868 u cwd DIR 1,17 832\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLsofCwd([]byte(tt.out))
			if tt.want == "" {
				if err == nil {
					t.Fatalf("parsed %q out of output with no usable cwd line", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLsofCwd: %v", err)
			}
			if got != tt.want {
				t.Errorf("cwd = %q, want %q", got, tt.want)
			}
		})
	}
}
