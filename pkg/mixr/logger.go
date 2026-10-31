package mixr

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zak-steenhoek/mixr/pkg/mixr/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	buildTypeNone    = ""
	buildTypeDev     = "dev"
	buildTypeRelease = "release"

	logDirectory = "logs"
	logFilename  = "mixr-latest-run.log"

	retainedRunLogs = 5
	runLogPrefix    = "mixr-run-"
	runLogTime      = "20060102-150405.000"
)

// NewLogger provides a logger instance for the whole program
func NewLogger(buildType string) (*zap.SugaredLogger, error) {
	var loggerConfig zap.Config

	// release: info and above, log to file only (no UI)
	if buildType == buildTypeRelease {
		if err := prepareReleaseLog(logDirectory, time.Now()); err != nil {
			return nil, fmt.Errorf("prepare release log: %w", err)
		}

		loggerConfig = zap.NewProductionConfig()

		loggerConfig.OutputPaths = []string{filepath.Join(logDirectory, logFilename)}
		loggerConfig.Encoding = "console"

		// development: debug and above, log to stderr only, colorful
	} else {
		loggerConfig = zap.NewDevelopmentConfig()

		// make it colorful
		loggerConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// all build types: make it readable
	loggerConfig.EncoderConfig.EncodeCaller = nil
	loggerConfig.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
	}

	loggerConfig.EncoderConfig.EncodeName = func(s string, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(fmt.Sprintf("%-27s", s))
	}

	logger, err := loggerConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("create zap logger: %w", err)
	}

	// no reason not to use the sugared logger - it's fast enough for anything we're gonna do
	sugar := logger.Sugar()

	return sugar, nil
}

func prepareReleaseLog(directory string, now time.Time) error {
	if err := util.EnsureDirExists(directory); err != nil {
		return fmt.Errorf("ensure log directory exists: %w", err)
	}

	currentPath := filepath.Join(directory, logFilename)
	if info, err := os.Stat(currentPath); err == nil && info.Size() > 0 {
		archiveName := runLogPrefix + now.Format(runLogTime) + ".log"
		if err := os.Rename(currentPath, filepath.Join(directory, archiveName)); err != nil {
			return fmt.Errorf("archive previous run log: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect current run log: %w", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("list log directory: %w", err)
	}
	archives := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), runLogPrefix) && strings.HasSuffix(entry.Name(), ".log") {
			archives = append(archives, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(archives)))
	for _, name := range archives[retainedCount(len(archives)):] {
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			return fmt.Errorf("remove expired run log %s: %w", name, err)
		}
	}
	return nil
}

func retainedCount(total int) int {
	if total < retainedRunLogs {
		return total
	}
	return retainedRunLogs
}
