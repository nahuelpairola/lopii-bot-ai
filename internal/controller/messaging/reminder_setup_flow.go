package messaging

import (
	"errors"
	"regexp"
	"strconv"
)

var errBadWindow = errors.New("reminder: bad window")

// twoHours grabs the first two 1–2 digit numbers in the text ("de 9 a 13",
// "20-22", "entre las 8 y las 10"). The leading (?:^|[^-\d]) requires each
// number to start clean (string start or a non-digit, non-minus char) so a
// negative like "-1" isn't misread as "1". ponytail: 24h whole-hours only, no
// am/pm NLP — the preset bands cover the common intent; upgrade only if users ask.
var twoHours = regexp.MustCompile(`(?:^|[^-\d])(\d{1,2})\D+(\d{1,2})`)

// parseWindow turns free text into a [start,end) window in minutes since
// midnight. Whole hours, 0–23, start strictly before end.
func parseWindow(text string) (startMin, endMin int, err error) {
	m := twoHours.FindStringSubmatch(text)
	if m == nil {
		return 0, 0, errBadWindow
	}
	h1, _ := strconv.Atoi(m[1])
	h2, _ := strconv.Atoi(m[2])
	if h1 < 0 || h1 > 23 || h2 < 0 || h2 > 23 || h1 >= h2 {
		return 0, 0, errBadWindow
	}
	return h1 * 60, h2 * 60, nil
}
