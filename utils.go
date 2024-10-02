package msgraph

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func NotIn(value interface{}, list ...interface{}) bool {

	for _, element := range list {
		if value == element {
			return false
		}
	}

	return true

}

func AnyToString(value any) (str string) {

	switch v := value.(type) {

	case int:
		str = strconv.Itoa(v)

	case int32:
		str = strconv.FormatInt(int64(v), 10)

	case int8, int16, int64:
		str = strconv.FormatInt(v.(int64), 10)

	case float32, float64:
		str = strconv.FormatFloat(v.(float64), byte('f'), -1, 64)

	case bool:
		str = strconv.FormatBool(v)

	case []string:
		str = strings.Join(v, ",")

	case string, []byte:
		str = v.(string)

	case sql.NullString:
		str = v.String

	case time.Time:
		str = v.Format(time.RFC3339)

	case nil:
		str = ""

	default:
		str, _ = ToJson(v)

	}

	return

}

func ToJson(data interface{}) (jsn string, err error) {

	var bytes []byte

	if bytes, err = json.Marshal(data); err == nil {
		jsn = string(bytes)
	}

	return

}
