package review

import "fmt"

// BuildNoChangesMessages generates context-specific user hints when git diff is empty.
func BuildNoChangesMessages(files []string, staged bool, branch, commit string) []string {
	if len(files) > 0 {
		return []string{
			"None of the requested files have diff content in the selected scope.",
			"Check the file paths or try running `scandrix review` without explicit files.",
		}
	}

	if branch != "" {
		return []string{
			fmt.Sprintf("No diff was found against `%s`.", branch),
			"Confirm the branch name or try a different base branch.",
		}
	}

	if commit != "" {
		return []string{
			fmt.Sprintf("No diff was found for commit `%s`.", commit),
			"Confirm the commit SHA or review a different revision.",
		}
	}

	if staged {
		return []string{
			"There are no staged changes to review.",
			"Stage files first or run `scandrix review` to inspect the full working tree.",
		}
	}

	return []string{
		"Try `scandrix review --staged` to review staged changes only.",
		"Or pass files explicitly, for example: `scandrix review src/file.go`.",
	}
}
