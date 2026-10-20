package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestConfigurationDocNamesExistInConfig(t *testing.T) {
	doc, err := os.ReadFile("../../docs/configuration.md")
	if err != nil {
		t.Fatal(err)
	}
	names := regexp.MustCompile(`FINFOCUS_[A-Z0-9_]+`).FindAllString(documentedSettings, -1)
	if len(names) == 0 {
		t.Fatal("config.go lists no documented settings")
	}
	for _, name := range names {
		if !strings.Contains(string(doc), name) {
			t.Errorf("%s is listed in config.go but missing from docs/configuration.md", name)
		}
	}
}
