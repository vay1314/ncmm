// Copyright (c) 2026 @3899. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAutoUpgradeConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ncmm_config_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tempDir)

	// Simulated old config.yaml (v1.0)
	oldYAML := `# 配置文件版本
version: 1.0

# 顶级多账号管理
accounts:
  # 音乐人主账号 Cookie 文件路径
  main: "/path/to/old/main/cookie.json"
  secondary:
    - "/path/to/old/fan1.json"

# log 日志模块配置
log:
  app: old_ncm
  level: debug

network:
  timeout: 30s
`

	testCfgPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(testCfgPath, []byte(oldYAML), 0644); err != nil {
		t.Fatalf("failed to write old config: %s", err)
	}

	// Run auto upgrade
	if err := AutoUpgradeConfig(testCfgPath); err != nil {
		t.Fatalf("AutoUpgradeConfig failed: %s", err)
	}

	// Read upgraded file
	upgradedData, err := os.ReadFile(testCfgPath)
	if err != nil {
		t.Fatalf("failed to read upgraded config: %s", err)
	}

	// Parse to verify structures
	var root yaml.Node
	if err := yaml.Unmarshal(upgradedData, &root); err != nil {
		t.Fatalf("failed to parse upgraded config: %s", err)
	}

	// Verify we got the new version from default template
	var conf Config
	if err := yaml.Unmarshal(upgradedData, &conf); err != nil {
		t.Fatalf("unmarshal Config struct failed: %s", err)
	}

	if conf.Version != defaultConfig.Version {
		t.Errorf("expected version to be upgraded to %s, got %s", defaultConfig.Version, conf.Version)
	}

	// Verify user-configured value was preserved
	if conf.Accounts == nil || conf.Accounts.Main != "/path/to/old/main/cookie.json" {
		t.Errorf("expected accounts.main to be preserved, got %+v", conf.Accounts)
	}
	if len(conf.Accounts.Secondary) != 1 || conf.Accounts.Secondary[0] != "/path/to/old/fan1.json" {
		t.Errorf("expected accounts.secondary to be preserved, got %+v", conf.Accounts.Secondary)
	}

	// Verify user-configured log value was preserved
	if conf.Log == nil || conf.Log.App != "old_ncm" || conf.Log.Level != "debug" {
		t.Errorf("expected log.app and log.level to be preserved, got %+v", conf.Log)
	}

	// Verify that new fields from the default template (like updater) were appended
	if conf.Updater == nil {
		t.Error("expected new config field 'updater' to be appended, but it was nil")
	} else {
		if conf.Updater.Check == nil || !*conf.Updater.Check {
			t.Error("expected updater.check default value (true) to be merged")
		}
		if conf.Updater.AutoUpdate == nil || *conf.Updater.AutoUpdate != *defaultConfig.Updater.AutoUpdate {
			t.Errorf("expected updater.auto_update default value (%t) to be merged, got %v", *defaultConfig.Updater.AutoUpdate, conf.Updater.AutoUpdate)
		}
	}
}
