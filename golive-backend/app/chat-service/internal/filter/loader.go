package filter

import (
	"bufio"
	"os"
	"strings"
)

// LoadWords reads one-word-per-line from path. Lines starting with `#` and
// blank lines are ignored. Whitespace is trimmed.
func LoadWords(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var words []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		words = append(words, line)
	}
	return words, sc.Err()
}
