package utils

import (
	"fmt"
	"strings"
)

func MaskOracleConnString(connStr string) string {
	if connStr == "" {
		return "--- EMPTY ---"
	}
	prefix := "oracle://"
	if !strings.HasPrefix(strings.ToLower(connStr), prefix) {
		return "*** UNKNOWN ORACLE CONN STRING FORMAT ***"
	}
	connWithoutPrefix := connStr[len(prefix):]
	atParts := strings.SplitN(connWithoutPrefix, "@", 2)
	if len(atParts) < 2 {
		slashParts := strings.SplitN(connWithoutPrefix, "/", 2)
		authPartCand := slashParts[0]
		colonPartsCand := strings.SplitN(authPartCand, ":", 2)
		if len(colonPartsCand) == 2 && colonPartsCand[1] != "" {
			return fmt.Sprintf("%s%s:***MASKED***%s", prefix, colonPartsCand[0], func() string {
				if len(slashParts) > 1 {
					return "/" + slashParts[1]
				}
				return ""
			}())
		}
		return connStr
	}
	authPart := atParts[0]
	hostPart := atParts[1]
	colonParts := strings.SplitN(authPart, ":", 2)
	user := colonParts[0]
	if len(colonParts) < 2 || colonParts[1] == "" {
		return fmt.Sprintf("%s%s@%s", prefix, user, hostPart)
	}
	return fmt.Sprintf("%s%s:***MASKED***@%s", prefix, user, hostPart)
}
