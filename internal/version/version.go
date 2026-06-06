package version

import "fmt"

// Values are replaced with linker flags in release builds.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// String returns the human-readable build version.
func String() string {
	if Commit == "" && Date == "" {
		return Version
	}
	return fmt.Sprintf("%s (commit=%s, built=%s)", Version, Commit, Date)
}
