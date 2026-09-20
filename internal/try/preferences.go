package try

import "errors"

// StorageKeyPreferences is the client storage key for view preferences.
const StorageKeyPreferences = "scandrix-review-prefs"

// DefaultPreferences returns standard UI viewing defaults.
func DefaultPreferences() Preferences {
	return Preferences{
		DiffStyle:         DiffStyleUnified,
		HideHighlights:    false,
		CollapseByDefault: false,
		FileTreeHidden:    false,
		FileTreeMode:      FileTreeModeGrouped,
	}
}

// ValidatePreferences ensures settings values adhere to allowed options.
func ValidatePreferences(prefs Preferences) error {
	if prefs.DiffStyle != DiffStyleSplit && prefs.DiffStyle != DiffStyleUnified {
		return errors.New("invalid diffStyle: must be 'split' or 'unified'")
	}
	if prefs.FileTreeMode != FileTreeModeTree && prefs.FileTreeMode != FileTreeModeGrouped {
		return errors.New("invalid fileTreeMode: must be 'tree' or 'grouped'")
	}
	return nil
}
