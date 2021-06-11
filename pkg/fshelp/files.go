package fshelp


import (
	"bufio"
	"os"
)


func ReadFileList(path string) (lines []string, err error) {
	file, err := os.Open(path)

	if err != nil {
		return
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err = scanner.Err(); err != nil {
		return
	}

	return
}
