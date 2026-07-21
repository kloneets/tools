package main

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"
)

var backupRuleDomains = []string{
	"root",
	"file",
	"database",
	"sharedpref",
	"external",
	"device_root",
	"device_file",
	"device_database",
	"device_sharedpref",
}

func TestPolicyURLs(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		wantPresent    []string
		wantNotPresent []string
	}{
		{
			name: "privacy html",
			path: "docs/privacy-policy.html",
			wantPresent: []string{
				"https://koko.lv/koko-tools/privacy-policy.html",
				"Koko Tools requests exclusion of its app-private Android data from Android cloud backup and device-to-device transfer.",
				"Device manufacturers and Android system components may control some migration behavior outside the app.",
				"Optional Firebase sync is separate from Android system backup and only runs if you enable it.",
				"janis@xit.lv",
			},
			wantNotPresent: []string{
				"https://koko.lv/privacy-policy.html",
				"open an issue",
			},
		},
		{
			name: "account deletion html",
			path: "docs/account-deletion.html",
			wantPresent: []string{
				"https://koko.lv/koko-tools/account-deletion.html",
				"janis@xit.lv",
			},
			wantNotPresent: []string{
				"https://koko.lv/account-deletion.html",
				"GitHub issue",
			},
		},
		{
			name: "privacy markdown",
			path: "PRIVACY.md",
			wantPresent: []string{
				"https://koko.lv/koko-tools/privacy-policy.html",
				"Koko Tools requests exclusion of its app-private Android data from Android cloud backup and device-to-device transfer.",
				"Device manufacturers and Android system components may control some migration behavior outside the app.",
				"Optional Firebase sync is separate from Android system backup and only runs if you enable it.",
				"janis@xit.lv",
			},
			wantNotPresent: []string{
				"https://koko.lv/privacy-policy.html",
				"open a GitHub issue",
			},
		},
		{
			name: "account deletion markdown",
			path: "ACCOUNT_DELETION.md",
			wantPresent: []string{
				"https://koko.lv/koko-tools/account-deletion.html",
				"janis@xit.lv",
			},
			wantNotPresent: []string{
				"https://koko.lv/account-deletion.html",
				"GitHub issue",
			},
		},
		{
			name: "android activity constants",
			path: "android/app/src/main/java/com/kloneets/kokotools/MainActivity.kt",
			wantPresent: []string{
				"https://koko.lv/koko-tools/privacy-policy.html",
				"https://koko.lv/koko-tools/account-deletion.html",
			},
			wantNotPresent: []string{
				"https://koko.lv/privacy-policy.html",
				"https://koko.lv/account-deletion.html",
			},
		},
		{
			name: "android manifest disables backup",
			path: "android/app/src/main/AndroidManifest.xml",
			wantPresent: []string{
				`android:allowBackup="false"`,
				`android:fullBackupContent="@xml/full_backup_content"`,
				`android:dataExtractionRules="@xml/data_extraction_rules"`,
			},
			wantNotPresent: []string{
				`android:allowBackup="true"`,
			},
		},
		{
			name: "release checklist",
			path: "android/RELEASE.md",
			wantPresent: []string{
				"https://koko.lv/koko-tools/privacy-policy.html",
				"https://koko.lv/koko-tools/account-deletion.html",
			},
			wantNotPresent: []string{
				"https://koko.lv/privacy-policy.html",
				"https://koko.lv/account-deletion.html",
			},
		},
		{
			name: "play publishing guide",
			path: "android/PLAY_STORE_PUBLISHING.md",
			wantPresent: []string{
				"https://koko.lv/koko-tools/privacy-policy.html",
				"https://koko.lv/koko-tools/account-deletion.html",
			},
			wantNotPresent: []string{
				"https://koko.lv/privacy-policy.html",
				"https://koko.lv/account-deletion.html",
				"managed assets",
			},
		},
		{
			name: "readme",
			path: "README.md",
			wantPresent: []string{
				"https://koko.lv/koko-tools/privacy-policy.html",
				"https://koko.lv/koko-tools/account-deletion.html",
			},
			wantNotPresent: []string{
				"https://koko.lv/privacy-policy.html",
				"https://koko.lv/account-deletion.html",
				"Managed file assets for notes.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentBytes, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read %s: %v", tt.path, err)
			}
			content := string(contentBytes)

			for _, want := range tt.wantPresent {
				if !strings.Contains(content, want) {
					t.Errorf("%s missing %q", tt.path, want)
				}
			}

			for _, stale := range tt.wantNotPresent {
				if strings.Contains(content, stale) {
					t.Errorf("%s contains stale text %q", tt.path, stale)
				}
			}
		})
	}
}

func TestAndroidBackupRulesExcludeAppData(t *testing.T) {
	fullBackupRules := readFullBackupContent(t, "android/app/src/main/res/xml/full_backup_content.xml")
	assertRuleSetExcludesDomains(t, "full backup content", fullBackupRules.Excludes)

	dataExtractionRules := readDataExtractionRules(t, "android/app/src/main/res/xml/data_extraction_rules.xml")
	assertRuleSetExcludesDomains(t, "cloud backup", dataExtractionRules.CloudBackup.Excludes)
	assertRuleSetExcludesDomains(t, "device transfer", dataExtractionRules.DeviceTransfer.Excludes)
}

type backupRule struct {
	Domain string `xml:"domain,attr"`
	Path   string `xml:"path,attr"`
}

type fullBackupContent struct {
	XMLName  xml.Name     `xml:"full-backup-content"`
	Excludes []backupRule `xml:"exclude"`
}

type dataExtractionRules struct {
	XMLName        xml.Name          `xml:"data-extraction-rules"`
	CloudBackup    backupRuleSection `xml:"cloud-backup"`
	DeviceTransfer backupRuleSection `xml:"device-transfer"`
}

type backupRuleSection struct {
	Excludes []backupRule `xml:"exclude"`
}

func readFullBackupContent(t *testing.T, path string) fullBackupContent {
	t.Helper()

	contentBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var rules fullBackupContent
	if err := xml.Unmarshal(contentBytes, &rules); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return rules
}

func readDataExtractionRules(t *testing.T, path string) dataExtractionRules {
	t.Helper()

	contentBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var rules dataExtractionRules
	if err := xml.Unmarshal(contentBytes, &rules); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return rules
}

func assertRuleSetExcludesDomains(t *testing.T, name string, rules []backupRule) {
	t.Helper()

	excluded := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.Path == "." {
			excluded[rule.Domain] = true
		}
	}

	for _, domain := range backupRuleDomains {
		if !excluded[domain] {
			t.Errorf("%s rules missing exclude for domain %q path %q", name, domain, ".")
		}
	}
}
