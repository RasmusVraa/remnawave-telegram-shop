package config

import (
	"testing"

	"github.com/google/uuid"
)

func TestTrialSquadsSettings(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	t.Cleanup(func() {
		_, _ = ApplyRuntimePatch(map[string]string{
			"TRIAL_INTERNAL_SQUADS":     "",
			"TRIAL_EXTERNAL_SQUAD_UUID": "",
		})
	})

	if _, err := ApplyRuntimePatch(map[string]string{"TRIAL_INTERNAL_SQUADS": a.String() + ", " + b.String()}); err != nil {
		t.Fatal(err)
	}
	got := TrialInternalSquads()
	if len(got) != 2 {
		t.Fatalf("ждали 2 сквада, получили %d", len(got))
	}
	if _, ok := got[a]; !ok {
		t.Fatalf("нет сквада %s", a)
	}

	// Битый UUID отклоняется, прежний выбор остаётся.
	if _, err := ApplyRuntimePatch(map[string]string{"TRIAL_INTERNAL_SQUADS": a.String() + ",не-uuid"}); err == nil {
		t.Fatal("битый UUID должен отклоняться")
	}
	if len(TrialInternalSquads()) != 2 {
		t.Fatal("после отклонённого сохранения прежний выбор должен остаться")
	}

	ext := uuid.New()
	if _, err := ApplyRuntimePatch(map[string]string{"TRIAL_EXTERNAL_SQUAD_UUID": ext.String()}); err != nil {
		t.Fatal(err)
	}
	if TrialExternalSquadUUID() != ext {
		t.Fatalf("внешний сквад: %s", TrialExternalSquadUUID())
	}
	if _, err := ApplyRuntimePatch(map[string]string{"TRIAL_EXTERNAL_SQUAD_UUID": "xyz"}); err == nil {
		t.Fatal("битый внешний UUID должен отклоняться")
	}

	// Пусто — откат на общие сквады.
	if _, err := ApplyRuntimePatch(map[string]string{"TRIAL_INTERNAL_SQUADS": ""}); err != nil {
		t.Fatal(err)
	}
	if len(conf.trialInternalSquads) != 0 {
		t.Fatal("пустой список должен очищать выбор")
	}
}

func TestTrialEnabledSwitch(t *testing.T) {
	oldDays := conf.trialDays
	t.Cleanup(func() {
		conf.trialDays = oldDays
		_, _ = ApplyRuntimePatch(map[string]string{"TRIAL_ENABLED": "true"})
	})
	conf.trialDays = 3

	if _, err := ApplyRuntimePatch(map[string]string{"TRIAL_ENABLED": "true"}); err != nil {
		t.Fatal(err)
	}
	if !TrialEnabled() {
		t.Fatal("включён и дни заданы — триал выдаётся")
	}
	if _, err := ApplyRuntimePatch(map[string]string{"TRIAL_ENABLED": "false"}); err != nil {
		t.Fatal(err)
	}
	if TrialEnabled() {
		t.Fatal("выключатель должен останавливать выдачу")
	}
	if TrialDays() != 3 {
		t.Fatal("дни при выключении не должны теряться")
	}
	// TRIAL_DAYS=0 из .env по-прежнему выключает триал.
	_, _ = ApplyRuntimePatch(map[string]string{"TRIAL_ENABLED": "true"})
	conf.trialDays = 0
	if TrialEnabled() {
		t.Fatal("нулевые дни выключают триал")
	}
}

func TestChannelGateScope(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_SCOPE": ChannelGateScopeAll})
	})
	if _, err := ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_SCOPE": "trial"}); err != nil {
		t.Fatal(err)
	}
	if !ChannelGateTrialOnly() {
		t.Fatal("область «только пробный» не применилась")
	}
	if _, err := ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_SCOPE": "nobody"}); err == nil {
		t.Fatal("неизвестная область должна отклоняться")
	}
	if _, err := ApplyRuntimePatch(map[string]string{"CHANNEL_GATE_SCOPE": "all"}); err != nil {
		t.Fatal(err)
	}
	if ChannelGateTrialOnly() {
		t.Fatal("область «всё» не применилась")
	}
}
