package secrets

import (
	"reflect"
	"strings"
	"testing"
)

func TestScanRules(t *testing.T) {
	hits := map[string]string{ // line → expected rule
		"aws_key = AKIAABCDEFGHIJKLMNOP":                    "AWS access key",
		"url = https://ghp_abcdefghijklmnop0123@github.com": "GitHub token",
		"GH=gho_abcdefghijklmnop0123":                       "GitHub token",
		"x glpat-abcdefghij0123456789":                      "GitLab token",
		"SONAR=squ_5316a0fa005ece102a80f4":                  "SonarQube token",
		"OPENAI sk-proj-abcdefghij0123456789":               "OpenAI-style key",
		"slack xoxb-1234567890-abcdef":                      "Slack token",
		"-----BEGIN OPENSSH PRIVATE KEY-----":               "private key",
		"//nexus:8084/repository/x/:_password=TS1YZXZ0IXRA": "password/token assignment",
		"password: hunter2":                                 "password/token assignment",
		`"api_token": "abc123"`:                             "password/token assignment",
	}
	for line, want := range hits {
		got := Scan([]Line{{File: "f", Num: 1, Text: line}})
		found := false
		for _, f := range got {
			found = found || f.Rule == want
		}
		if !found {
			t.Errorf("%q: rules %v, want %q", line, rulesOf(got), want)
		}
	}
}

func TestScanIgnoresNonSecrets(t *testing.T) {
	for _, line := range []string{
		`token = ""`,
		"password=$PASSWORD",
		"secret: ${SECRET}",
		"password = <your password here>",
		"see the task-manager docs",
		"ghp_short",
		"the token is rotated monthly",
		"alias c=clear",
	} {
		if got := Scan([]Line{{File: "f", Num: 1, Text: line}}); len(got) > 0 {
			t.Errorf("%q flagged: %v", line, rulesOf(got))
		}
	}
}

func TestExcerptMasksSecret(t *testing.T) {
	secret := "squ_5316a0fa005ece102a80f4e72f51e9a8a0775a6d"
	got := Scan([]Line{{File: ".zshrc", Num: 3, Text: "export SONARQUBE_TOKEN=" + secret}})
	if len(got) == 0 {
		t.Fatal("no finding")
	}
	for _, f := range got {
		if strings.Contains(f.Excerpt, secret[8:]) {
			t.Errorf("excerpt leaks the secret: %q", f.Excerpt)
		}
		if f.File != ".zshrc" || f.Num != 3 {
			t.Errorf("location = %s:%d", f.File, f.Num)
		}
	}
}

func TestParseDiff(t *testing.T) {
	diff := `diff --git a/Configs/zsh/.zshrc b/Configs/zsh/.zshrc
index 1..2 100644
--- a/Configs/zsh/.zshrc
+++ b/Configs/zsh/.zshrc
@@ -10,0 +11,2 @@
+export A=1
+export B=2
@@ -20 +22 @@
-old
+new
diff --git a/gone b/gone
--- a/gone
+++ /dev/null
@@ -1 +0,0 @@
-bye
diff --git a/new file b/new file
--- /dev/null
+++ b/new file
@@ -0,0 +1 @@
+hello
`
	want := []Line{
		{File: "Configs/zsh/.zshrc", Num: 11, Text: "export A=1"},
		{File: "Configs/zsh/.zshrc", Num: 12, Text: "export B=2"},
		{File: "Configs/zsh/.zshrc", Num: 22, Text: "new"},
		{File: "new file", Num: 1, Text: "hello"},
	}
	if got := ParseDiff(diff); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseDiff =\n%+v\nwant\n%+v", got, want)
	}
}

func rulesOf(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return out
}
