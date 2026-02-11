package cupl

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pborges/cupl/examples"
	"github.com/pborges/cupl/internal/jed"
	"github.com/pborges/cupl/internal/testutil"
)

func TestBlackbox(t *testing.T) {
	pldFiles, err := fs.Glob(examples.FS, "*.PLD")
	if err != nil {
		t.Fatal(err)
	}
	lower, err := fs.Glob(examples.FS, "*.pld")
	if err != nil {
		t.Fatal(err)
	}
	pldFiles = append(pldFiles, lower...)

	if len(pldFiles) == 0 {
		t.Fatal("no PLD files found in examples FS")
	}

	for _, pldPath := range pldFiles {
		ext := filepath.Ext(pldPath)
		name := strings.TrimSuffix(pldPath, ext)
		jedPath := name + ".jed"

		t.Run(name, func(t *testing.T) {
			pld := mustRead(t, pldPath)
			expected := mustRead(t, jedPath)
			content, err := Parse(pld)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			g, err := Compile(content)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			gotJed := jed.MakeJEDEC(jed.Config{}, g)
			compareJEDEC(t, gotJed, expected)
		})
	}
}

func compareJEDEC(t *testing.T, gotJed string, expected []byte) {
	got, err := testutil.ParseJEDEC([]byte(gotJed))
	if err != nil {
		t.Fatalf("parse got jed: %v", err)
	}
	want, err := testutil.ParseJEDEC(expected)
	if err != nil {
		t.Fatalf("parse expected jed: %v", err)
	}
	if diff := testutil.CompareJEDEC(got, want); diff != "" {
		t.Fatalf("%s", diff)
	}
}

func mustRead(t *testing.T, path string) []byte {
	b, err := examples.FS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
