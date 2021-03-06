package project

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	log "github.com/golang/glog"
)

func isNum(c uint8) bool {
	return c >= '0' && c <= '9'
}

func validVariableNameChar(c uint8) bool {
	return c == '_' ||
		c >= 'A' && c <= 'Z' ||
		c >= 'a' && c <= 'z' ||
		isNum(c)
}

func parseVariable(line string, pos int, mapping func(string) string) (string, int, bool) {
	var buffer bytes.Buffer

	for ; pos < len(line); pos++ {
		c := line[pos]

		switch {
		case validVariableNameChar(c):
			buffer.WriteByte(c)
		default:
			return mapping(buffer.String()), pos - 1, true
		}
	}

	return mapping(buffer.String()), pos, true
}

func parseVariableWithBraces(line string, pos int, mapping func(string) string) (string, int, bool) {
	var buffer bytes.Buffer

	for ; pos < len(line); pos++ {
		c := line[pos]

		switch {
		case c == '}':
			bufferString := buffer.String()

			if bufferString == "" {
				return "", 0, false
			}

			return mapping(buffer.String()), pos, true
		case validVariableNameChar(c):
			buffer.WriteByte(c)
		default:
			return "", 0, false
		}
	}

	return "", 0, false
}

func parseInterpolationExpression(line string, pos int, mapping func(string) string) (string, int, bool) {
	c := line[pos]

	switch {
	case c == '$':
		return "$", pos, true
	case c == '{':
		return parseVariableWithBraces(line, pos+1, mapping)
	case !isNum(c) && validVariableNameChar(c):
		// Variables can't start with a number
		return parseVariable(line, pos, mapping)
	default:
		return "", 0, false
	}
}

func parseLine(line string, mapping func(string) string) (string, bool) {
	var buffer bytes.Buffer

	for pos := 0; pos < len(line); pos++ {
		c := line[pos]
		switch {
		case c == '$':
			var replaced string
			var success bool

			replaced, pos, success = parseInterpolationExpression(line, pos+1, mapping)

			if !success {
				return "", false
			}

			buffer.WriteString(replaced)
		default:
			buffer.WriteByte(c)
		}
	}

	return buffer.String(), true
}

func parseConfig(option, service string, data *interface{}, mapping func(string) string) error {
	switch typedData := (*data).(type) {
	case string:
		var success bool

		interpolatedLine, success := parseLine(typedData, mapping)

		if !success {
			return fmt.Errorf("Invalid interpolation format for \"%s\" option in service \"%s\": \"%s\"", option, service, typedData)
		}

		// If possible, convert the value to an integer
		// If the type should be a string and not an int, go-yaml will convert it back into a string
		lineAsInteger, err := strconv.Atoi(interpolatedLine)

		if err == nil {
			*data = lineAsInteger
		} else {
			*data = interpolatedLine
		}
	case []interface{}:
		for k, v := range typedData {
			err := parseConfig(option, service, &v, mapping)

			if err != nil {
				return err
			}

			typedData[k] = v
		}
	case map[interface{}]interface{}:
		for k, v := range typedData {
			err := parseConfig(option, service, &v, mapping)

			if err != nil {
				return err
			}

			typedData[k] = v
		}
	}

	return nil
}

func interpolate(environmentLookup EnvironmentLookup, config *rawServiceMap) error {
	for k, v := range *config {
		for k2, v2 := range v {
			err := parseConfig(k2, k, &v2, func(s string) string {
				values := environmentLookup.Lookup(s, k, nil)

				if len(values) == 0 {
					log.Warningf("The %s variable is not set. Substituting a blank string.", s)
					return ""
				}

				// Use first result if many are given
				value := values[0]

				// Environment variables come in key=value format
				// Return everything past first '='
				return strings.SplitN(value, "=", 2)[1]
			})

			if err != nil {
				return err
			}

			(*config)[k][k2] = v2
		}
	}

	return nil
}
