package lucide

import "os"

// readFile returns the first n bytes of a file.
func readFile(name string, n int) (string, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	if len(data) > n {
		data = data[:n]
	}
	return string(data), nil
}
