package main

import (
	"errors"
	"testing"
)

func TestDataLayerChecker(t *testing.T) {
	tests := []struct {
		name         string
		files        []string
		allowedFiles []string
		fileContents map[string]string
		wantErr      bool
	}{
		{
			name:         "detects import from @/convex/_generated/api",
			files:        []string{"src/app.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/app.tsx": `import { api } from "@/convex/_generated/api";`,
			},
			wantErr: true,
		},
		{
			name:         "detects import from convex/react",
			files:        []string{"src/hooks.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/hooks.ts": `import { useQuery } from "convex/react";`,
			},
			wantErr: true,
		},
		{
			name:         "detects useMutation from convex/react",
			files:        []string{"src/actions.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/actions.tsx": `import { useMutation } from "convex/react";`,
			},
			wantErr: true,
		},
		{
			name:         "detects useAction from convex/react",
			files:        []string{"src/actions.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/actions.tsx": `import { useAction } from "convex/react";`,
			},
			wantErr: true,
		},
		{
			name:         "detects multiple imports from convex/react",
			files:        []string{"src/component.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/component.tsx": `import { useQuery, useMutation, useAction } from "convex/react";`,
			},
			wantErr: true,
		},
		{
			name:         "detects single quotes",
			files:        []string{"src/app.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/app.tsx": `import { api } from '@/convex/_generated/api';`,
			},
			wantErr: true,
		},
		{
			name:         "allowed files are skipped",
			files:        []string{"packages/data-layer/src/hooks.ts", "src/app.tsx"},
			allowedFiles: []string{"packages/data-layer/"},
			fileContents: map[string]string{
				"packages/data-layer/src/hooks.ts": `import { useQuery } from "convex/react";`,
				"src/app.tsx":                      `import { useDataLayerQuery } from "@/data-layer";`,
			},
			wantErr: false,
		},
		{
			name:         "backend files allowed",
			files:        []string{"packages/backend/convex/functions.ts"},
			allowedFiles: []string{"packages/backend/"},
			fileContents: map[string]string{
				"packages/backend/convex/functions.ts": `import { api } from "@/convex/_generated/api";`,
			},
			wantErr: false,
		},
		{
			name:         "non-JS/TS files are skipped",
			files:        []string{"config.json", "README.md", "styles.css"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"config.json": `{"import": "from convex/react"}`,
				"README.md":   `Use import { useQuery } from "convex/react"`,
				"styles.css":  `.convex { display: none; }`,
			},
			wantErr: false,
		},
		{
			name:         "clean files pass",
			files:        []string{"src/clean.ts", "src/pure.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/clean.ts": `import { useQuery } from "@/data-layer";`,
				"src/pure.tsx": `import { api } from "@/data-layer/api";`,
			},
			wantErr: false,
		},
		{
			name:         "multiple violations detected",
			files:        []string{"src/a.tsx", "src/b.tsx", "src/c.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/a.tsx": `import { api } from "@/convex/_generated/api";`,
				"src/b.tsx": `import { useQuery } from "convex/react";`,
				"src/c.ts":  `import { useMutation } from "convex/react";`,
			},
			wantErr: true,
		},
		{
			name:         "mixed allowed and violations",
			files:        []string{"packages/data-layer/src/index.ts", "src/violation.tsx"},
			allowedFiles: []string{"packages/data-layer/"},
			fileContents: map[string]string{
				"packages/data-layer/src/index.ts": `import { useQuery } from "convex/react";`,
				"src/violation.tsx":                `import { useQuery } from "convex/react";`,
			},
			wantErr: true,
		},
		{
			name:         "empty file list",
			files:        []string{},
			allowedFiles: nil,
			fileContents: map[string]string{},
			wantErr:      false,
		},
		{
			name:         "git show error handled gracefully",
			files:        []string{"missing.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{},
			wantErr:      false,
		},
		{
			name:         "file with both violations",
			files:        []string{"src/page.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/page.tsx": `import { api } from "@/convex/_generated/api";
import { useQuery } from "convex/react";
const data = useQuery(api.users.list);`,
			},
			wantErr: true,
		},
		{
			name:         "similar but different import path is not flagged",
			files:        []string{"src/app.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/app.tsx": `import { api } from "@/convex/_generated/api_helpers";`,
			},
			wantErr: false,
		},
		{
			name:         "convex/react-native is not flagged",
			files:        []string{"src/app.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/app.tsx": `import { something } from "convex/react-native";`,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &DataLayerChecker{
				gitShowFunc: func(file string) ([]byte, error) {
					content, ok := tt.fileContents[file]
					if !ok {
						return nil, errors.New("file not found")
					}
					return []byte(content), nil
				},
			}

			err := checker.Check("test", tt.files, tt.allowedFiles)
			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDataLayerIsCheckableFile(t *testing.T) {
	checker := NewDataLayerChecker()

	tests := []struct {
		file string
		want bool
	}{
		{"src/app.ts", true},
		{"src/component.tsx", true},
		{"src/util.js", true},
		{"src/widget.jsx", true},
		{"config.json", false},
		{"README.md", false},
		{"styles.css", false},
		{"Makefile", false},
		{"script.sh", false},
		{".ts", true},
		{"", false},
		{"file.typescript", false},
		{"file.ts.bak", false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := checker.isCheckableFile(tt.file)
			if got != tt.want {
				t.Errorf("isCheckableFile(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestDataLayerIsAllowedFile(t *testing.T) {
	checker := NewDataLayerChecker()

	tests := []struct {
		name         string
		file         string
		allowedFiles []string
		want         bool
	}{
		{
			name:         "file in allowed list",
			file:         "packages/backend/convex/api.ts",
			allowedFiles: []string{"packages/backend/", "packages/data-layer/"},
			want:         true,
		},
		{
			name:         "file not in allowed list",
			file:         "src/app.tsx",
			allowedFiles: []string{"packages/backend/"},
			want:         false,
		},
		{
			name:         "empty allowed list",
			file:         "src/app.tsx",
			allowedFiles: []string{},
			want:         false,
		},
		{
			name:         "nil allowed list",
			file:         "src/app.tsx",
			allowedFiles: nil,
			want:         false,
		},
		{
			name:         "data-layer directory allowed",
			file:         "packages/data-layer/src/generated-hooks.ts",
			allowedFiles: []string{"packages/data-layer/"},
			want:         true,
		},
		{
			name:         "substring pattern matches",
			file:         "apps/web/packages/backend/test.ts",
			allowedFiles: []string{"packages/backend/"},
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.isAllowedFile(tt.file, tt.allowedFiles)
			if got != tt.want {
				t.Errorf("isAllowedFile(%q, %v) = %v, want %v", tt.file, tt.allowedFiles, got, tt.want)
			}
		})
	}
}

func TestHasDataLayerViolations(t *testing.T) {
	checker := NewDataLayerChecker()

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			"import api from generated",
			`import { api } from "@/convex/_generated/api";`,
			true,
		},
		{
			"import useQuery from convex/react",
			`import { useQuery } from "convex/react";`,
			true,
		},
		{
			"import useMutation from convex/react",
			`import { useMutation } from "convex/react";`,
			true,
		},
		{
			"import useAction from convex/react",
			`import { useAction } from "convex/react";`,
			true,
		},
		{
			"import multiple hooks from convex/react",
			`import { useQuery, useMutation } from "convex/react";`,
			true,
		},
		{
			"single quotes",
			`import { api } from '@/convex/_generated/api';`,
			true,
		},
		{
			"clean import from data-layer",
			`import { useQuery } from "@/data-layer";`,
			false,
		},
		{
			"no imports at all",
			`const x = 42;`,
			false,
		},
		{
			"empty content",
			"",
			false,
		},
		{
			"similar path not matched",
			`import { api } from "@/convex/_generated/api_v2";`,
			false,
		},
		{
			"convex/react substring not matched",
			`import { something } from "convex/react-native";`,
			false,
		},
		{
			"multiline with violation",
			"import React from 'react';\nimport { api } from \"@/convex/_generated/api\";\n",
			true,
		},
		{
			"import in comment still matched",
			`// import { api } from "@/convex/_generated/api";`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.hasDataLayerViolations([]byte(tt.content))
			if got != tt.want {
				t.Errorf("hasDataLayerViolations(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}
