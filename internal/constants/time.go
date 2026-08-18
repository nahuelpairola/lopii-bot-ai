package constants

import "time"

// ArgentinaZone is the wall clock every user-facing date in this app is
// computed in. Fixed UTC-3 instead of time.LoadLocation: Argentina observes no
// DST, so a fixed offset avoids depending on the IANA tz database being
// present on the host (Alpine/scratch images ship without it).
// ponytail: fixed -3; if Argentina ever restores DST, switch to
// time.LoadLocation + embedded time/tzdata.
var ArgentinaZone = time.FixedZone("ART", -3*60*60)

// MonthLongEs are the Spanish month names, indexed by time.Month()-1. Shared
// because two surfaces render a month to the user: the Mini App period header
// and the weekly summary.
var MonthLongEs = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
