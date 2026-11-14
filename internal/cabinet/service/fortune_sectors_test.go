package service

import (
	"testing"

	cabcfg "remnawave-tg-shop-bot/internal/cabinet/config"
)

func testWheelCfg() cabcfg.FortuneWheelConfig {
	return cabcfg.FortuneWheelConfig{
		WeightMicro: 478, WeightXP: 355, WeightDiscount3: 60, WeightDays3: 25, WeightDiscount5: 40,
		WeightDays5: 15, WeightDays7: 10, WeightDays15: 5, WeightDays30: 2, WeightDays180: 0, WeightFreeSpin: 10,
		RewardFreeSpins: 1, RewardDays180: 180,
	}
}

func sectorTypes(s []FortuneSectorDTO) map[string]FortuneSectorDTO {
	m := map[string]FortuneSectorDTO{}
	for _, x := range s {
		m[x.RewardType] = x
	}
	return m
}

func TestBuildSectorsZeroWeightHidden(t *testing.T) {
	svc := &FortuneService{}
	cfg := testWheelCfg()

	got := sectorTypes(svc.buildSectors(cfg))
	if len(got) != 11 {
		t.Fatalf("all 11 sectors expected (180 stays on the wheel), got %d", len(got))
	}
	if got["free_spin"].DisplaySpins != 1 {
		t.Errorf("free_spin display spins = %d", got["free_spin"].DisplaySpins)
	}

	cfg.WeightFreeSpin = 0
	cfg.WeightDays30 = 0
	got = sectorTypes(svc.buildSectors(cfg))
	for _, rt := range []string{"free_spin", "days_30"} {
		if _, ok := got[rt]; ok {
			t.Errorf("%s has weight 0 and must not be on the wheel", rt)
		}
	}
	if _, ok := got["days_180"]; !ok {
		t.Error("days_180 must stay on the wheel even with weight 0")
	}
	if len(got) != 9 {
		t.Errorf("expected 9 sectors, got %d", len(got))
	}
	// индексы остаются исходными, чтобы sector_index из ответа на спин совпадал с сектором
	if got["days_15"].Index != fortuneSectorIndex("days_15") {
		t.Errorf("sector index shifted: %d", got["days_15"].Index)
	}
}

func TestPickRewardTypeNoFreeSpinChain(t *testing.T) {
	svc := &FortuneService{}
	cfg := cabcfg.FortuneWheelConfig{WeightFreeSpin: 1000, WeightXP: 1}
	for i := 0; i < 300; i++ {
		rt, err := svc.pickRewardType(cfg, true)
		if err != nil {
			t.Fatal(err)
		}
		if rt == "free_spin" {
			t.Fatal("a bonus spin must not win another free spin")
		}
	}
	seen := false
	for i := 0; i < 300 && !seen; i++ {
		rt, _ := svc.pickRewardType(cfg, false)
		seen = rt == "free_spin"
	}
	if !seen {
		t.Error("a regular spin should be able to win free_spin")
	}
}

func TestZeroWeightPrizeNeverPicked(t *testing.T) {
	svc := &FortuneService{}
	cfg := testWheelCfg()
	cfg.WeightFreeSpin = 0
	for i := 0; i < 2000; i++ {
		rt, err := svc.pickRewardType(cfg, false)
		if err != nil {
			t.Fatal(err)
		}
		if rt == "free_spin" || rt == "days_180" {
			t.Fatalf("zero-weight prize dropped: %s", rt)
		}
	}
}
