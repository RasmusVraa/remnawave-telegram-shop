package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Сквады пробного периода: TRIAL_INTERNAL_SQUADS (список UUID через запятую) и
// TRIAL_EXTERNAL_SQUAD_UUID (один UUID). Пусто — берутся общие SQUAD_UUIDS и
// EXTERNAL_SQUAD_UUID, а если пусты и они, то все внутренние сквады панели.

// applyTrialInternalSquads разбирает список UUID из админки или .env.
func applyTrialInternalSquads(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		conf.trialInternalSquads = nil
		return nil
	}
	squads := make(map[uuid.UUID]uuid.UUID)
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := uuid.Parse(part)
		if err != nil {
			return fmt.Errorf("invalid UUID %q", part)
		}
		squads[id] = id
	}
	if len(squads) == 0 {
		conf.trialInternalSquads = nil
		return nil
	}
	conf.trialInternalSquads = squads
	return nil
}

// currentTrialInternalSquads — выбранные сквады в стабильном порядке, чтобы
// админка не видела «изменений» там, где поменялся лишь порядок обхода карты.
func currentTrialInternalSquads() string {
	ids := make([]string, 0, len(conf.trialInternalSquads))
	for id := range conf.trialInternalSquads {
		ids = append(ids, id.String())
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func applyTrialExternalSquad(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		conf.trialExternalSquadUUID = uuid.Nil
		return nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid UUID")
	}
	conf.trialExternalSquadUUID = id
	return nil
}

func uuidOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
