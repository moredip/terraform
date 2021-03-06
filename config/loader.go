package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Load loads the Terraform configuration from a given file.
//
// This file can be any format that Terraform recognizes, and import any
// other format that Terraform recognizes.
func Load(path string) (*Config, error) {
	importTree, err := loadTree(path)
	if err != nil {
		return nil, err
	}

	configTree, err := importTree.ConfigTree()

	// Close the importTree now so that we can clear resources as quickly
	// as possible.
	importTree.Close()

	if err != nil {
		return nil, err
	}

	return configTree.Flatten()
}

// LoadDir loads all the Terraform configuration files in a single
// directory and appends them together.
//
// Special files known as "override files" can also be present, which
// are merged into the loaded configuration. That is, the non-override
// files are loaded first to create the configuration. Then, the overrides
// are merged into the configuration to create the final configuration.
//
// Files are loaded in lexical order.
func LoadDir(root string) (*Config, error) {
	files, overrides, err := dirFiles(root)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf(
			"No Terraform configuration files found in directory: %s",
			root)
	}

	// Determine the absolute path to the directory.
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var result *Config

	// Sort the files and overrides so we have a deterministic order
	sort.Strings(files)
	sort.Strings(overrides)

	// Load all the regular files, append them to each other.
	for _, f := range files {
		c, err := Load(f)
		if err != nil {
			return nil, err
		}

		if result != nil {
			result, err = Append(result, c)
			if err != nil {
				return nil, err
			}
		} else {
			result = c
		}
	}

	// Load all the overrides, and merge them into the config
	for _, f := range overrides {
		c, err := Load(f)
		if err != nil {
			return nil, err
		}

		result, err = Merge(result, c)
		if err != nil {
			return nil, err
		}
	}

	// Mark the directory
	result.Dir = rootAbs

	return result, nil
}

// IsEmptyDir returns true if the directory given has no Terraform
// configuration files.
func IsEmptyDir(root string) (bool, error) {
	if _, err := os.Stat(root); err != nil && os.IsNotExist(err) {
		return true, nil
	}

	fs, os, err := dirFiles(root)
	if err != nil {
		return false, err
	}

	return len(fs) == 0 && len(os) == 0, nil
}

// Ext returns the Terraform configuration extension of the given
// path, or a blank string if it is an invalid function.
func ext(path string) string {
	if strings.HasSuffix(path, ".tf") {
		return ".tf"
	} else if strings.HasSuffix(path, ".tf.json") {
		return ".tf.json"
	} else {
		return ""
	}
}

func dirFiles(dir string) ([]string, []string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !fi.IsDir() {
		return nil, nil, fmt.Errorf(
			"configuration path must be a directory: %s",
			dir)
	}

	var files, overrides []string
	err = nil
	for err != io.EOF {
		var fis []os.FileInfo
		fis, err = f.Readdir(128)
		if err != nil && err != io.EOF {
			return nil, nil, err
		}

		for _, fi := range fis {
			// Ignore directories
			if fi.IsDir() {
				continue
			}

			// Only care about files that are valid to load
			name := fi.Name()
			extValue := ext(name)
			if extValue == "" {
				continue
			}

			// Determine if we're dealing with an override
			nameNoExt := name[:len(name)-len(extValue)]
			override := nameNoExt == "override" ||
				strings.HasSuffix(nameNoExt, "_override")

			path := filepath.Join(dir, name)
			if override {
				overrides = append(overrides, path)
			} else {
				files = append(files, path)
			}
		}
	}

	return files, overrides, nil
}
