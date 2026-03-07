package accounting

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const microusdPerUSD int64 = 1_000_000

// NormalizeUSDDecimal trims, validates, and canonicalizes one USD decimal
// string while returning its exact microusd representation.
func NormalizeUSDDecimal(raw string) (string, int64, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", 0, fmt.Errorf("must be a non-empty decimal string")
	}
	if strings.ContainsAny(clean, "eE+-") {
		return "", 0, fmt.Errorf("must use plain base-10 decimal notation")
	}

	parts := strings.Split(clean, ".")
	if len(parts) > 2 {
		return "", 0, fmt.Errorf("must contain at most one decimal point")
	}
	if parts[0] == "" {
		return "", 0, fmt.Errorf("must include whole-dollar digits before the decimal point")
	}
	if len(parts) == 2 && parts[1] == "" {
		return "", 0, fmt.Errorf("must include fractional digits when a decimal point is present")
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return "", 0, fmt.Errorf("must contain digits only")
			}
		}
	}

	fractional := ""
	if len(parts) == 2 {
		fractional = parts[1]
		if len(fractional) > 6 {
			return "", 0, fmt.Errorf("must use at most 6 fractional digits")
		}
	}

	wholeUSD, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("must fit within signed 64-bit microusd precision; %w", err)
	}
	if wholeUSD > math.MaxInt64/microusdPerUSD {
		return "", 0, fmt.Errorf("must fit within signed 64-bit microusd precision")
	}

	fractionPadded := fractional + strings.Repeat("0", 6-len(fractional))
	fractionMicrousd := int64(0)
	if fractionPadded != "" {
		fractionMicrousd, err = strconv.ParseInt(fractionPadded, 10, 64)
		if err != nil {
			return "", 0, fmt.Errorf("must fit within signed 64-bit microusd precision; %w", err)
		}
	}

	microusd := wholeUSD * microusdPerUSD
	if fractionMicrousd > math.MaxInt64-microusd {
		return "", 0, fmt.Errorf("must fit within signed 64-bit microusd precision")
	}
	microusd += fractionMicrousd
	if microusd <= 0 {
		return "", 0, fmt.Errorf("must be greater than 0")
	}

	canonicalWhole := strings.TrimLeft(parts[0], "0")
	if canonicalWhole == "" {
		canonicalWhole = "0"
	}
	canonicalFraction := strings.TrimRight(fractional, "0")
	if canonicalFraction == "" {
		return canonicalWhole, microusd, nil
	}
	return canonicalWhole + "." + canonicalFraction, microusd, nil
}

// FormatMicrousdAsUSD returns the canonical USD decimal form for one microusd
// value.
func FormatMicrousdAsUSD(microusd int64) string {
	if microusd == 0 {
		return "0"
	}

	wholeUSD := microusd / microusdPerUSD
	fractionMicrousd := microusd % microusdPerUSD
	if fractionMicrousd == 0 {
		return strconv.FormatInt(wholeUSD, 10)
	}

	fraction := strings.TrimRight(fmt.Sprintf("%06d", fractionMicrousd), "0")
	return strconv.FormatInt(wholeUSD, 10) + "." + fraction
}
