package dashboard

import (
	"cmp"
	_ "embed"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed pricing.json
var officialPricing []byte

type price struct {
	Input  *float64 `json:"inputCostPerToken"`
	Cached *float64 `json:"cacheReadCostPerToken"`
	Write  *float64 `json:"cacheWriteCostPerToken"`
	Output *float64 `json:"outputCostPerToken"`
	Source string   `json:"source"`
}
type priceBook struct {
	UpdatedAt string           `json:"updatedAt"`
	Models    map[string]price `json:"models"`
}
type currency struct {
	Code string  `json:"code"`
	Rate float64 `json:"rate"`
}

var modelDate = regexp.MustCompile(`-\d{8}$`)

func loadPrices(home, override string) (priceBook, error) {
	var book priceBook
	if err := json.Unmarshal(officialPricing, &book); err != nil {
		return book, err
	}
	var cached struct {
		Data map[string]price `json:"data"`
	}
	if err := readJSON(filepath.Join(home, ".cache/codeburn/litellm-pricing.json"), &cached); err != nil && !errors.Is(err, os.ErrNotExist) {
		return book, err
	}
	for name, p := range cached.Data {
		if _, known := book.Models[name]; !known {
			book.Models[name] = p
		}
	}
	if override != "" {
		var custom priceBook
		if err := readJSON(override, &custom); err != nil {
			return book, err
		}
		if len(custom.Models) == 0 {
			return book, errors.New("价格文件缺少 models")
		}
		for name, p := range custom.Models {
			if err := p.validate(); err != nil {
				return book, fmt.Errorf("模型 %s 价格无效: %w", name, err)
			}
			book.Models[name] = p
		}
		if custom.UpdatedAt != "" {
			book.UpdatedAt = custom.UpdatedAt
		}
	}
	return book, nil
}
func (p price) validate() error {
	if p.Input == nil || p.Output == nil {
		return errors.New("缺少输入或输出价格")
	}
	for _, rate := range []*float64{p.Input, p.Cached, p.Write, p.Output} {
		if rate != nil && (*rate < 0 || math.IsNaN(*rate) || math.IsInf(*rate, 0)) {
			return errors.New("价格必须为非负有限数")
		}
	}
	return nil
}
func (b priceBook) cost(e event) (float64, bool) {
	normalized := e.Model
	if _, tail, ok := strings.CutLast(normalized, "/"); ok {
		normalized = tail
	}
	normalized, _, _ = strings.Cut(normalized, "@")
	normalized = modelDate.ReplaceAllString(normalized, "")
	p, ok := b.Models[e.Model]
	if !ok {
		p, ok = b.Models[normalized]
	}
	if !ok || p.validate() != nil {
		return 0, false
	}
	total := 0.0
	for _, term := range []struct {
		n    int64
		rate *float64
	}{{e.Input, p.Input}, {e.Cached, p.Cached}, {e.Write, p.Write}, {e.Output, p.Output}} {
		if term.n == 0 {
			continue
		}
		if term.rate == nil {
			return 0, false
		}
		total += float64(term.n) * *term.rate
	}
	return total, true
}
func loadCurrency(home string) currency {
	usd := currency{"USD", 1}
	var config struct {
		Currency struct {
			Code string `json:"code"`
		} `json:"currency"`
	}
	if readJSON(cmp.Or(os.Getenv("CODEBURN_CFG"), filepath.Join(home, ".config/codeburn/config.json")), &config) != nil {
		return usd
	}
	code := strings.ToUpper(config.Currency.Code)
	if code == "USD" || !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(code) {
		return usd
	}
	var fx currency
	if readJSON(cmp.Or(os.Getenv("FX_CACHE"), filepath.Join(home, ".cache/codeburn/exchange-rate.json")), &fx) != nil || strings.ToUpper(fx.Code) != code || fx.Rate <= 0 {
		return usd
	}
	return currency{code, fx.Rate}
}
