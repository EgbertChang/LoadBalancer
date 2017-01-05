package parseConf

import (
	"io/ioutil"
	"bytes"
	"regexp"
)

// ParseJSON方法接受一个需要解析的文件路径作为参数
func ParseJSON(filePath string) ([]byte, error) {

	confFile_bytes, err :=ioutil.ReadFile(filePath)
	if err != nil {
		return []byte(""), err
	}

	confFile_bytes=bytes.Replace(confFile_bytes, []byte("\r"), []byte(""), -1)
	lines := bytes.Split(confFile_bytes, []byte("\n"))
	slice_confFile_bytes := make([][]byte, 0)
	reg := regexp.MustCompile("(\\s*#.*\\n?)|(\\s*\\n?)")

	for _, line := range lines {
		trim_line := reg.ReplaceAll(line, []byte(""))
		slice_confFile_bytes = append(slice_confFile_bytes, trim_line)
	}
	trim_confFile_bytes := bytes.Join(slice_confFile_bytes, []byte(""))

	return trim_confFile_bytes, nil
}
