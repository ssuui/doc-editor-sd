package recordstore

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

func yamlUnmarshal(in []byte, out any) error {
	return yaml.Unmarshal(in, out)
}

func strconvFormat(v uint64) string {
	return strconv.FormatUint(v, 10)
}
