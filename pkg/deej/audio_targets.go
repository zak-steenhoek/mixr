package deej

import (
	"strings"

	"github.com/omriharel/deej/pkg/deej/util"
	"github.com/thoas/go-funk"
)

func resolveAudioTargetNames(target string, unmappedSessions []Session) []string {
	target = strings.ToLower(target)
	if !strings.HasPrefix(target, specialTargetTransformPrefix) {
		return []string{target}
	}
	return resolveSpecialAudioTarget(strings.TrimPrefix(target, specialTargetTransformPrefix), unmappedSessions)
}

func resolveSpecialAudioTarget(specialTargetName string, unmappedSessions []Session) []string {
	switch specialTargetName {
	case specialTargetCurrentWindow:
		currentWindowProcessNames, err := util.GetCurrentWindowProcessNames()
		if err != nil {
			return nil
		}
		for index, target := range currentWindowProcessNames {
			currentWindowProcessNames[index] = strings.ToLower(target)
		}
		return funk.UniqString(currentWindowProcessNames)

	case specialTargetAllUnmapped:
		targetKeys := make([]string, len(unmappedSessions))
		for index, session := range unmappedSessions {
			targetKeys[index] = session.Key()
		}
		return targetKeys
	}
	return nil
}
