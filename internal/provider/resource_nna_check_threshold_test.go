package provider

import "testing"

// TestThresholdPattern pins the Nagios range-syntax grammar accepted for
// warning_threshold/critical_threshold. It is deliberately broader than the
// regex Network Analyzer's own UI uses (which rejects negative and
// fractional bounds), so that a check already stored with such a threshold
// can be imported and then still expressed in configuration.
func TestThresholdPattern(t *testing.T) {
	valid := []string{
		"1000",    // plain max
		"0",       // zero
		"10:20",   // explicit range
		"10:",     // open-ended upper
		":20",     // open-ended lower
		"~:500",   // negative infinity to 500
		"@10:20",  // inverted range
		"@~:500",  // inverted, negative infinity
		"-5:5",    // negative lower bound
		"-5",      // bare negative
		"0.5",     // fractional
		"1.5:2.5", // fractional range
		"@-5:-1",  // inverted negative range
	}
	for _, in := range valid {
		if !thresholdPattern.MatchString(in) || !thresholdHasValue.MatchString(in) {
			t.Errorf("threshold %q should be accepted", in)
		}
	}

	invalid := []string{
		"",         // expressed by omitting the attribute instead
		"@",        // no value at all
		"~",        // lower bound with no range
		"abc",      // not a number
		"10:20:30", // too many bounds
		"1,000",    // thousands separator
		"10 : 20",  // spaces
		"~1000",    // "~" is only meaningful as a range's lower bound
		"1e5",      // exponent notation
	}
	for _, in := range invalid {
		if thresholdPattern.MatchString(in) && thresholdHasValue.MatchString(in) {
			t.Errorf("threshold %q should be rejected", in)
		}
	}
}
