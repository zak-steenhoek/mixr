package mixr

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/thoas/go-funk"
)

type sliderMap struct {
	m    map[int][]string
	lock sync.Locker
}

func newSliderMap() *sliderMap {
	return &sliderMap{
		m:    make(map[int][]string),
		lock: &sync.Mutex{},
	}
}

func sliderMapFromConfigs(userMapping map[string][]string, internalMapping map[string][]string) *sliderMap {
	resultMap := newSliderMap()

	// copy targets from user config, ignoring empty values
	for sliderIdxString, targets := range userMapping {
		sliderIdx, err := strconv.Atoi(sliderIdxString)
		if err != nil || sliderIdx < 0 {
			continue
		}

		resultMap.set(sliderIdx, cleanConfigTargets(targets))
	}

	// add targets from internal configs, ignoring duplicate or empty values
	for sliderIdxString, targets := range internalMapping {
		sliderIdx, err := strconv.Atoi(sliderIdxString)
		if err != nil || sliderIdx < 0 {
			continue
		}

		existingTargets, ok := resultMap.get(sliderIdx)
		if !ok {
			existingTargets = []string{}
		}

		filteredTargets := funk.FilterString(cleanConfigTargets(targets), func(s string) bool {
			return !funk.ContainsString(existingTargets, s)
		})

		existingTargets = append(existingTargets, filteredTargets...)
		resultMap.set(sliderIdx, existingTargets)
	}

	return resultMap
}

func cleanConfigTargets(targets []string) []string {
	cleaned := make([]string, 0, len(targets))
	for _, target := range targets {
		if target = strings.TrimSpace(target); target != "" {
			cleaned = append(cleaned, target)
		}
	}
	return cleaned
}

func (m *sliderMap) iterate(f func(int, []string)) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for key, value := range m.m {
		f(key, value)
	}
}

func (m *sliderMap) get(key int) ([]string, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	value, ok := m.m[key]
	return value, ok
}

func (m *sliderMap) set(key int, value []string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.m[key] = value
}

// replace publishes a complete, newly constructed mapping in one transaction.
func (m *sliderMap) replace(next *sliderMap) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.m = next.m
}

func (m *sliderMap) String() string {
	m.lock.Lock()
	defer m.lock.Unlock()

	sliderCount := 0
	targetCount := 0

	for _, value := range m.m {
		sliderCount++
		targetCount += len(value)
	}

	return fmt.Sprintf("<%d sliders mapped to %d targets>", sliderCount, targetCount)
}
