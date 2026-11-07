package mixr

import (
	"strings"

	"github.com/thoas/go-funk"
	"github.com/zak-steenhoek/mixr/pkg/mixr/util"
)

var currentWindowProcessNames = util.GetCurrentWindowProcessNames

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
		processNames, err := currentWindowProcessNames()
		if err != nil {
			return nil
		}
		for index, target := range processNames {
			processNames[index] = strings.ToLower(target)
		}
		return funk.UniqString(processNames)

	case specialTargetAllUnmapped:
		targetKeys := make([]string, len(unmappedSessions))
		for index, session := range unmappedSessions {
			targetKeys[index] = session.Key()
		}
		return targetKeys
	}
	return nil
}
