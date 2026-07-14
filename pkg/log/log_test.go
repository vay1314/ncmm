// Copyright (c) 2026 @3899. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package log

import (
	"fmt"
	"log/slog"
	"testing"
)

func init() {
	Default = New(nil)
}

func TestPrint(t *testing.T) {
	Debug("hello debug")
	Info("hello info:%s", "chaunsin")
	InfoW(fmt.Sprintf("hello info:%s", "chaunsin"), "sex", slog.StringValue("man"))

	Default.SetLevel(slog.LevelWarn)
	Info("can not print")
	Fatal("hello fatal")
}

func TestLineWriter(t *testing.T) {
	Default = New(&Config{
		App:    "test",
		Format: "text",
		Level:  "info",
		Stdout: false,
		Rotate: defaultConfig.Rotate,
	})
	w := LineWriter().(*lineWriter)
	if _, err := w.Write([]byte("[sign] hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := w.Write([]byte("[sign] partial")); err != nil {
		t.Fatalf("Write partial: %v", err)
	}
	w.Flush()
	if len(w.buf) != 0 {
		t.Fatalf("expected empty buffer after Flush, got %q", w.buf)
	}
}
