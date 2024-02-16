package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutor(t *testing.T) {
	tmp := t.TempDir()
	for _, test := range []struct {
		name         string
		input        string
		outDir       string
		outFormats   []string
		wantOutDir   string
		wantOutFiles []string
	}{
		{
			name:         "input is a dir and all files are processed",
			input:        "./testdata",
			outDir:       "",
			outFormats:   []string{"mobi"},
			wantOutDir:   "./testdata/wordwise_generated",
			wantOutFiles: []string{"./testdata/wordwise_generated/test-wordwise-gen1.mobi", "./testdata/wordwise_generated/test-wordwise-gen2.mobi"},
		},
		{
			name:         "input is a file and only that file is processed",
			input:        "./testdata/test-wordwise-gen1.epub",
			outDir:       tmp,
			outFormats:   []string{"epub"},
			wantOutDir:   tmp,
			wantOutFiles: []string{path.Join(tmp, "test-wordwise-gen1.epub")},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			exe, err := newExecutor(1000, 5, 4, test.input, test.outDir, "wordwise_generated", test.outFormats)
			if err != nil {
				t.Fatalf("newExecutor: %v", err)
			}
			err = exe.generateWordwise()
			if err != nil {
				t.Fatalf("exe.generateWordwise: %v", err)
			}
			for _, file := range test.wantOutFiles {
				_, err = os.Stat(file)
				if errors.Is(err, os.ErrNotExist) {
					t.Fatalf("expected output file %s doesn't exist!", file)
				}
				err = os.Remove(file)
				if err != nil {
					t.Fatalf("remove generated file failed: %v", err)
				}
			}
		})
	}
}

func TestListAllChildFilesIfInputIsADir(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "input is a dir and all child files are listed except for generatedFolderName",
			input: "./testdata",
			want:  []string{"testdata/test-wordwise-gen1.epub", "testdata/test-wordwise-gen2.epub"},
		},
		{
			name:  "input is a file and only that file is listed",
			input: "./testdata/test-wordwise-gen1.epub",
			want:  []string{"./testdata/test-wordwise-gen1.epub"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			exe, err := newExecutor(1000, 5, 4, test.input, "", "wordwise_generated", nil)
			if err != nil {
				t.Fatalf("newExecutor: %v", err)
			}
			got, rErr := exe.listAllChildFilesIfInputIsADir()
			if rErr != nil {
				t.Fatalf("listAllChildFilesIfInputIsADir: %v", rErr)
			}
			assert.ElementsMatch(t, test.want, got, fmt.Sprintf("want %+v, got %+v", test.want, got))
		})
	}
}

func TestReplacementMediator(t *testing.T) {
	m := newReplacementMediator(10)
	word := "doesntmatter"
	if m.hasReplacedJustNow(word, 20) {
		t.Fatalf("no, %s hasn't been replaced before", word)
	}
	m.setLastReplacedPosition(word, 1)
	if !m.hasReplacedJustNow(word, 8) {
		t.Fatalf("%s has just been replaced at 1. next postion must be >= 11", word)
	}
	if m.hasReplacedJustNow(word, 12) {
		t.Fatalf("%s was replaced at 1 is expected to be replaced at postion >= 11 (12 > 11)", word)
	}
}
