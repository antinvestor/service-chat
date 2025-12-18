package internal

import (
	"fmt"
)

func MetadataKey(profileID, deviceID string) string {
	return fmt.Sprintf("%s:%s", profileID, deviceID)
}
