package adapters

import "fmt"

// parseInt parses s as a decimal integer and returns the parsed value.
// It returns an error if the input does not contain a valid decimal integer.
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}


