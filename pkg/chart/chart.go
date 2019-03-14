package chart

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
)

// ReplaceConfig replaces orginal config with value in the key of given path.
func ReplaceConfig(origin, path, newValue, suffix string) (string, error) {
	paths := strings.Split(path, "/")
	if len(paths) == 0 {
		return "", fmt.Errorf("path is empty")
	}

	var result []byte
	var err error

	// get _config from newValue
	config, _, _, err := jsonparser.Get([]byte(newValue), "_config")
	if err != nil {
		return "", err
	}

	// the first path is useless, skip it
	paths = append(paths[1:], "_config")
	temp, _, _, err := jsonparser.Get([]byte(origin), paths...)
	if err == nil {
		revision, nerr := jsonparser.GetInt(temp, "_metadata", "revision")
		if nerr == jsonparser.KeyPathNotFoundError {
			revision = 2
		} else {
			revision++
		}
		newConfig, nerr := jsonparser.Set(config, []byte(strconv.Itoa(int(revision))), "_metadata", "revision")
		if nerr != nil {
			return "", nerr
		}

		// change controller name if exists
		controllerName, nerr := jsonparser.GetString(newConfig, "controllers", "[0]", "controller", "name")
		if nerr != nil && nerr != jsonparser.KeyPathNotFoundError {
			return "", nerr
		}
		if controllerName != "" && nerr == nil {
			// quoted by ""
			newName := fmt.Sprintf("\"%s\"", rebuildControllerName(controllerName, suffix))
			newConfig, nerr = jsonparser.Set(newConfig, []byte(newName), "controllers", "[0]", "controller", "name")
			if nerr != nil {
				return "", nerr
			}
		}
		result, err = jsonparser.Set([]byte(origin), newConfig, paths...)
	}
	return string(result), err
}

func rebuildControllerName(name, suffix string) string {
	slice := strings.Split(name, "-")
	if len(slice) == 1 {
		return fmt.Sprintf("%s-%s", name, suffix)
	}
	existed := slice[len(slice)-1]
	if len(existed) != len(suffix) {
		return fmt.Sprintf("%s-%s", name, suffix)
	}
	slice[len(slice)-1] = suffix
	return strings.Join(slice, "-")
}
