//go:build contract

// Контрактный тест против живой панели Remnawave 3.3.x–3.4.x.
//
// Юнит-тесты не ловят то, ради чего затевалась миграция: неверный путь,
// разъехавшийся JSON-тег, безтелесный 204. Здесь проверяется настоящий клиент
// против настоящей панели.
//
// Не гоняется обычным `go test ./...` — нужен тег `contract` и живой стенд.
//
// Сборка под стенд и запуск:
//
//	GOOS=linux GOARCH=amd64 go test -c -tags contract ./internal/remnawave/ -o rw_contract.test
//	# залить на сервер, затем на сервере:
//	RW_CONTRACT_URL=http://127.0.0.1:3100 RW_CONTRACT_TOKEN=<api-token> ./rw_contract.test -test.v
package remnawave

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"remnawave-tg-shop-bot/utils"
	"testing"
	"time"

	"github.com/google/uuid"
)

func contractClient(t *testing.T) *Client {
	t.Helper()
	base := os.Getenv("RW_CONTRACT_URL")
	token := os.Getenv("RW_CONTRACT_TOKEN")
	if base == "" || token == "" {
		t.Skip("RW_CONTRACT_URL / RW_CONTRACT_TOKEN не заданы — контрактный тест пропущен")
	}
	// mode=local включает x-forwarded-*, без которых ProxyCheckMiddleware
	// панели 3.x рвёт соединение без ответа.
	return NewClient(base, token, "local")
}

// TestContractUserLifecycle проходит весь путь, который ломался в 3.0.0.
func TestContractUserLifecycle(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	telegramID := time.Now().UnixNano()%1_000_000_000 + 900_000_000_000
	username := fmt.Sprintf("contract_%d", time.Now().Unix())
	expire := time.Now().UTC().AddDate(0, 0, 30)

	tl := int64(0)
	// Напрямую через doJSON, а не через createUser: тот тянет сквады из config,
	// который в контрактном тесте не инициализирован.
	var createResp apiResponse[User]
	err := c.doJSON(ctx, http.MethodPost, "/api/users", &CreateUserRequest{
		Username:             username,
		ExpireAt:             expire,
		Status:               "ACTIVE",
		TrafficLimitBytes:    &tl,
		TrafficLimitStrategy: "MONTH",
		TelegramID:           &telegramID,
	}, &createResp)
	if err != nil {
		t.Fatalf("создание пользователя: %v", err)
	}
	created := &createResp.Response
	if created.ID <= 0 {
		t.Fatalf("панель вернула пустой id: %+v", created)
	}
	t.Logf("создан id=%d username=%s shortUuid=%s", created.ID, created.Username, created.ShortUUID)

	// Прибираем за собой даже если тест упадёт на середине.
	defer func() {
		if err := c.DeleteUser(ctx, created.ID); err != nil {
			t.Errorf("удаление пользователя (DELETE отвечает 204 без тела): %v", err)
		} else {
			t.Log("DELETE 204 без тела обработан корректно")
		}
	}()

	t.Run("stream по telegramId заменяет удалённый by-telegram-id", func(t *testing.T) {
		found, err := c.getUsersByTelegramID(ctx, telegramID)
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		if len(found) != 1 {
			t.Fatalf("ожидался 1 пользователь, пришло %d", len(found))
		}
		if found[0].ID != created.ID {
			t.Fatalf("stream вернул чужой профиль: %d != %d", found[0].ID, created.ID)
		}
	})

	t.Run("GetUserByID по числовому пути", func(t *testing.T) {
		got, err := c.GetUserByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetUserByID: %v", err)
		}
		if got.Username != username {
			t.Fatalf("username разъехался: %q != %q", got.Username, username)
		}
		if got.SubscriptionUrl == "" {
			t.Error("subscriptionUrl пуст — подозрение на разъехавшийся JSON-тег")
		}
	})

	t.Run("extend продлевает от expireAt у активной подписки", func(t *testing.T) {
		before, err := c.GetUserByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("чтение до продления: %v", err)
		}
		after, err := c.ExtendUserDays(ctx, created.ID, 7)
		if err != nil {
			t.Fatalf("ExtendUserDays: %v", err)
		}
		want := before.ExpireAt.AddDate(0, 0, 7)
		if diff := after.ExpireAt.Sub(want); diff > time.Minute || diff < -time.Minute {
			t.Fatalf("expireAt=%s, ожидалось ~%s", after.ExpireAt, want)
		}
	})

	// Самый денежный кейс: заблокированный клиент оплачивает продление.
	// Голый extend оставляет DISABLED/LIMITED как есть, поэтому в
	// ExtendSubscriptionByDaysPreserveSquads добавлен добивающий PATCH статуса.
	t.Run("продление оживляет заблокированного клиента", func(t *testing.T) {
		if _, err := c.PatchUser(ctx, &UpdateUserRequest{ID: &created.ID, Status: "DISABLED"}); err != nil {
			t.Fatalf("блокировка: %v", err)
		}
		revived, err := c.ExtendSubscriptionByDaysPreserveSquads(ctx, 1, telegramID, 3)
		if err != nil {
			t.Fatalf("ExtendSubscriptionByDaysPreserveSquads: %v", err)
		}
		if revived.Status != "ACTIVE" {
			t.Fatalf("клиент остался %s — оплатил продление и сидит без доступа", revived.Status)
		}
	})

	t.Run("HWID по числовому userId", func(t *testing.T) {
		devices, err := c.GetUserDevices(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetUserDevices: %v", err)
		}
		t.Logf("устройств: %d (пусто — норма, нода не подключена)", len(devices))
	})

	t.Run("PATCH лимита трафика по id", func(t *testing.T) {
		want := int64(1024 * 1024 * 1024)
		got, err := c.PatchUser(ctx, &UpdateUserRequest{ID: &created.ID, TrafficLimitBytes: &want})
		if err != nil {
			t.Fatalf("PatchUser: %v", err)
		}
		if got.TrafficLimitBytes != want {
			t.Fatalf("лимит не применился: %d != %d", got.TrafficLimitBytes, want)
		}
	})

	t.Run("revoke выдаёт новую ссылку", func(t *testing.T) {
		before, err := c.GetUserByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("чтение до revoke: %v", err)
		}
		after, err := c.RevokeUserSubscription(ctx, created.ID)
		if err != nil {
			t.Fatalf("RevokeUserSubscription: %v", err)
		}
		if after.ShortUUID == "" || after.ShortUUID == before.ShortUUID {
			t.Fatalf("shortUuid не сменился: %q -> %q", before.ShortUUID, after.ShortUUID)
		}
		if after.SubscriptionUrl == "" || after.SubscriptionUrl == before.SubscriptionUrl {
			t.Fatalf("subscriptionUrl не сменился: %q -> %q", before.SubscriptionUrl, after.SubscriptionUrl)
		}
		if err := c.DeleteAllUserDevices(ctx, created.ID); err != nil {
			t.Fatalf("DeleteAllUserDevices: %v", err)
		}
	})

	t.Run("сброс трафика", func(t *testing.T) {
		if err := c.ResetUserTraffic(ctx, created.ID); err != nil {
			t.Fatalf("ResetUserTraffic: %v", err)
		}
	})
}

// TestContractInternalSquads проверяет, что сквады остались на UUID.
func TestContractInternalSquads(t *testing.T) {
	c := contractClient(t)
	squads, err := c.ListInternalSquads(context.Background())
	if err != nil {
		t.Fatalf("ListInternalSquads: %v", err)
	}
	t.Logf("сквадов: %d", len(squads))
	for _, s := range squads {
		if s.Name == "" {
			t.Errorf("сквад %s без имени — разъехался JSON-тег", s.UUID)
		}
	}
}

// TestContractNotFound: 404 должен приходить как ErrNotFound, а не как «битый JSON».
func TestContractNotFound(t *testing.T) {
	c := contractClient(t)
	_, err := c.GetUserByID(context.Background(), 999_999_999)
	if err == nil {
		t.Fatal("ожидалась ошибка для несуществующего id")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound, получено: %v", err)
	}
}

// TestContractStreamPagination — курсорная пагинация на объёме больше одной страницы.
//
// Размер страницы в streamUsers — 250. Ошибка в курсоре не падает, а молча
// обрывает выдачу: сломались бы синхронизация, админ-поиск и резолв web-only
// клиентов (он делает полный обход). Поэтому проверяем на 300 пользователях.
func TestContractStreamPagination(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	const total = 300
	stamp := time.Now().Unix()
	created := make([]int64, 0, total)

	defer func() {
		for _, id := range created {
			if err := c.DeleteUser(ctx, id); err != nil {
				t.Errorf("не удалось удалить %d: %v", id, err)
			}
		}
	}()

	expire := time.Now().UTC().AddDate(0, 0, 5)
	tl := int64(0)
	for i := 0; i < total; i++ {
		var resp apiResponse[User]
		err := c.doJSON(ctx, http.MethodPost, "/api/users", &CreateUserRequest{
			Username:             fmt.Sprintf("pg_%d_%03d", stamp, i),
			ExpireAt:             expire,
			Status:               "ACTIVE",
			TrafficLimitBytes:    &tl,
			TrafficLimitStrategy: "MONTH",
		}, &resp)
		if err != nil {
			t.Fatalf("создание #%d: %v", i, err)
		}
		created = append(created, resp.Response.ID)
	}
	t.Logf("создано %d пользователей", len(created))

	all, err := c.GetUsers(ctx)
	if err != nil {
		t.Fatalf("GetUsers: %v", err)
	}

	seen := make(map[int64]int, len(all))
	for _, u := range all {
		seen[u.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("пользователь %d встретился %d раз — курсор повторяет страницу", id, n)
		}
	}
	missing := 0
	for _, id := range created {
		if seen[id] == 0 {
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("%d из %d созданных не вернулись — пагинация обрывается", missing, total)
	}
	t.Logf("всего в панели %d, все %d созданных на месте, дублей нет", len(all), total)
}

// TestContractWebOnlyFallback — резолв клиента без реального telegram_id.
//
// Самый хрупкий путь: stream по telegramId ничего не находит, и клиент ищется
// полным обходом по префиксу "<customer_id>_". Ломается молча — web-клиенту
// просто «не найдётся» его подписка.
func TestContractWebOnlyFallback(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	customerID := time.Now().Unix()%100000 + 400000
	syntheticTG := utils.SyntheticTelegramID(customerID)
	if !utils.IsSyntheticTelegramID(syntheticTG) {
		t.Fatalf("%d не считается synthetic — тест проверяет не тот путь", syntheticTG)
	}

	username := fmt.Sprintf("%d_web", customerID)
	tl := int64(0)
	var resp apiResponse[User]
	err := c.doJSON(ctx, http.MethodPost, "/api/users", &CreateUserRequest{
		Username:             username,
		ExpireAt:             time.Now().UTC().AddDate(0, 0, 10),
		Status:               "ACTIVE",
		TrafficLimitBytes:    &tl,
		TrafficLimitStrategy: "MONTH",
		// telegramId намеренно не задаём: у web-only клиента его нет.
	}, &resp)
	if err != nil {
		t.Fatalf("создание web-only профиля: %v", err)
	}
	created := resp.Response
	defer func() { _ = c.DeleteUser(ctx, created.ID) }()

	found, err := c.findExistingUserForCustomer(ctx, customerID, syntheticTG)
	if err != nil {
		t.Fatalf("findExistingUserForCustomer: %v", err)
	}
	if found == nil {
		t.Fatal("web-only клиент не найден — fallback по префиксу username не сработал")
	}
	if found.ID != created.ID {
		t.Fatalf("найден чужой профиль: %d != %d", found.ID, created.ID)
	}
}

// TestContractAdminPatchOnBlockedUser — регрессия на статус в PATCH.
//
// Админка читает карточку и шлёт её обратно вместе с правкой, включая текущий
// статус. LIMITED и EXPIRED панель 3.x на запись не принимает (400), поэтому
// клиент обязан их вычищать. Без этого правка тега у клиента с превышенным
// трафиком падала бы с ошибкой.
func TestContractAdminPatchOnBlockedUser(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	tl := int64(0)
	var resp apiResponse[User]
	err := c.doJSON(ctx, http.MethodPost, "/api/users", &CreateUserRequest{
		Username:             fmt.Sprintf("blocked_%d", time.Now().UnixNano()%1000000),
		ExpireAt:             time.Now().UTC().AddDate(0, 0, -5),
		Status:               "LIMITED",
		TrafficLimitBytes:    &tl,
		TrafficLimitStrategy: "MONTH",
	}, &resp)
	if err != nil {
		t.Fatalf("создание заблокированного: %v", err)
	}
	u := resp.Response
	defer func() { _ = c.DeleteUser(ctx, u.ID) }()

	if u.Status != "LIMITED" {
		t.Skipf("панель не отдала LIMITED (получено %s) — кейс не воспроизводится", u.Status)
	}

	// Ровно то, что делает админка: правка поля + эхо текущего статуса.
	tag := "ADMINTAG"
	patched, err := c.PatchUser(ctx, &UpdateUserRequest{ID: &u.ID, Status: u.Status, Tag: &tag})
	if err != nil {
		t.Fatalf("правка тега у LIMITED-клиента упала: %v", err)
	}
	if patched.Tag == nil || *patched.Tag != tag {
		t.Fatalf("тег не применился: %v", patched.Tag)
	}
	if patched.Status == "ACTIVE" {
		t.Error("правка тега сняла блокировку — статус не должен был меняться")
	}
}

// TestContractTariffPurchaseAndRenewal — основной путь покупки: выдача по профилю
// тарифа (свои сквады, лимит трафика, лимит устройств, тег), затем продление тем же
// тарифом. Проверяет, что PATCH по числовому id применяет всё разом и что при
// продлении сквады не теряются.
func TestContractTariffPurchaseAndRenewal(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	squads, err := c.ListInternalSquads(ctx)
	if err != nil {
		t.Fatalf("ListInternalSquads: %v", err)
	}
	if len(squads) == 0 {
		t.Skip("в панели нет internal squads — тарифный профиль проверить нечем")
	}
	want := squads[0]

	customerID := time.Now().Unix()%100000 + 500000
	telegramID := time.Now().UnixNano()%1_000_000_000 + 910_000_000_000
	profile := TariffPaidProfile{
		TrafficLimitBytes:         5 * 1024 * 1024 * 1024,
		TrafficLimitResetStrategy: "MONTH",
		SquadUUIDs:                []uuid.UUID{want.UUID},
		Tag:                       "CONTRACT",
		BaseDeviceLimit:           3,
	}

	created, err := c.CreateOrUpdateUserWithTariffProfile(ctx, customerID, telegramID, 30, profile)
	if err != nil {
		t.Fatalf("покупка по тарифу: %v", err)
	}
	defer func() { _ = c.DeleteUser(ctx, created.ID) }()

	if created.TrafficLimitBytes != profile.TrafficLimitBytes {
		t.Errorf("лимит трафика: %d, ожидался %d", created.TrafficLimitBytes, profile.TrafficLimitBytes)
	}
	if created.HwidDeviceLimit == nil || *created.HwidDeviceLimit != profile.BaseDeviceLimit {
		t.Errorf("лимит устройств: %v, ожидался %d", created.HwidDeviceLimit, profile.BaseDeviceLimit)
	}
	if created.Tag == nil || *created.Tag != profile.Tag {
		t.Errorf("тег: %v, ожидался %q", created.Tag, profile.Tag)
	}
	if len(created.ActiveInternalSquads) != 1 || created.ActiveInternalSquads[0].UUID != want.UUID {
		t.Fatalf("сквады выданы неверно: %+v, ожидался только %s", created.ActiveInternalSquads, want.UUID)
	}

	// Продление тем же тарифом: срок растёт, сквады остаются.
	renewed, err := c.CreateOrUpdateUserWithTariffProfile(ctx, customerID, telegramID, 30, profile)
	if err != nil {
		t.Fatalf("продление по тарифу: %v", err)
	}
	if !renewed.ExpireAt.After(created.ExpireAt) {
		t.Fatalf("срок не вырос: было %s, стало %s", created.ExpireAt, renewed.ExpireAt)
	}
	if len(renewed.ActiveInternalSquads) != 1 || renewed.ActiveInternalSquads[0].UUID != want.UUID {
		t.Fatalf("продление потеряло сквады: %+v", renewed.ActiveInternalSquads)
	}
	if renewed.ID != created.ID {
		t.Fatalf("продление создало новый профиль: %d != %d", renewed.ID, created.ID)
	}
}

// TestContractDeviceLimitChange — докупка HWID меняет лимит устройств у профиля,
// найденного по customer/telegram.
func TestContractDeviceLimitChange(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	customerID := time.Now().Unix()%100000 + 600000
	telegramID := time.Now().UnixNano()%1_000_000_000 + 920_000_000_000
	profile := TariffPaidProfile{
		TrafficLimitBytes:         0,
		TrafficLimitResetStrategy: "MONTH",
		BaseDeviceLimit:           2,
	}
	u, err := c.CreateOrUpdateUserWithTariffProfile(ctx, customerID, telegramID, 15, profile)
	if err != nil {
		t.Fatalf("подготовка профиля: %v", err)
	}
	defer func() { _ = c.DeleteUser(ctx, u.ID) }()

	updated, err := c.UpdateUserDeviceLimitByCustomer(ctx, customerID, telegramID, 7)
	if err != nil {
		t.Fatalf("UpdateUserDeviceLimitByCustomer: %v", err)
	}
	if updated.HwidDeviceLimit == nil || *updated.HwidDeviceLimit != 7 {
		t.Fatalf("лимит устройств не применился: %v", updated.HwidDeviceLimit)
	}
	if updated.ID != u.ID {
		t.Fatalf("изменён чужой профиль: %d != %d", updated.ID, u.ID)
	}
}

// TestContractFortuneShrink — списание дней колесом фортуны.
// Идёт не через панельный extend (тот принимает только положительные дни),
// а через PATCH, поэтому проверяется отдельно.
func TestContractFortuneShrink(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	customerID := time.Now().Unix()%100000 + 700000
	telegramID := time.Now().UnixNano()%1_000_000_000 + 930_000_000_000
	u, err := c.CreateOrUpdateUserWithTariffProfile(ctx, customerID, telegramID, 30, TariffPaidProfile{
		TrafficLimitBytes:         0,
		TrafficLimitResetStrategy: "MONTH",
	})
	if err != nil {
		t.Fatalf("подготовка профиля: %v", err)
	}
	defer func() { _ = c.DeleteUser(ctx, u.ID) }()

	shrunk, err := c.ShrinkSubscriptionByDaysPreserveSquads(ctx, customerID, telegramID, 5)
	if err != nil {
		t.Fatalf("ShrinkSubscriptionByDaysPreserveSquads: %v", err)
	}
	want := u.ExpireAt.AddDate(0, 0, -5)
	if diff := shrunk.ExpireAt.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Fatalf("срок после списания %s, ожидался ~%s", shrunk.ExpireAt, want)
	}

	// Начисление дней фортуной идёт общим путём продления.
	won, err := c.ExtendSubscriptionByDaysPreserveSquads(ctx, customerID, telegramID, 3)
	if err != nil {
		t.Fatalf("начисление дней: %v", err)
	}
	if !won.ExpireAt.After(shrunk.ExpireAt) {
		t.Fatalf("срок не вырос после выигрыша: было %s, стало %s", shrunk.ExpireAt, won.ExpireAt)
	}
}

// TestContractPing — health-check бота дёргает панель; если он отвалится,
// /health встанет в 503, хотя бот исправен. Единственный путь к users,
// оставшийся на offset-пагинации: в 3.3 GET /api/users?start&size жив,
// и это надо подтверждать, а не предполагать.
func TestContractPing(t *testing.T) {
	if err := contractClient(t).Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// TestContractStreamFilterIsHonoured — фильтр stream по telegramId обязан
// действительно фильтровать. Если панель его проигнорирует, клиент вернёт
// чужие профили, и продление/удаление уйдут на посторонний аккаунт.
func TestContractStreamFilterIsHonoured(t *testing.T) {
	c := contractClient(t)
	ctx := context.Background()

	tgA := time.Now().UnixNano()%1_000_000_000 + 940_000_000_000
	tgB := tgA + 1
	tl := int64(0)
	ids := make([]int64, 0, 2)
	defer func() {
		for _, id := range ids {
			_ = c.DeleteUser(ctx, id)
		}
	}()

	for i, tg := range []int64{tgA, tgB} {
		tgCopy := tg
		var resp apiResponse[User]
		err := c.doJSON(ctx, http.MethodPost, "/api/users", &CreateUserRequest{
			Username:             fmt.Sprintf("flt_%d_%d", time.Now().Unix(), i),
			ExpireAt:             time.Now().UTC().AddDate(0, 0, 5),
			Status:               "ACTIVE",
			TrafficLimitBytes:    &tl,
			TrafficLimitStrategy: "MONTH",
			TelegramID:           &tgCopy,
		}, &resp)
		if err != nil {
			t.Fatalf("создание #%d: %v", i, err)
		}
		ids = append(ids, resp.Response.ID)
	}

	found, err := c.getUsersByTelegramID(ctx, tgA)
	if err != nil {
		t.Fatalf("getUsersByTelegramID: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("ожидался ровно 1 профиль, пришло %d — фильтр не сработал", len(found))
	}
	if found[0].TelegramID == nil || *found[0].TelegramID != tgA {
		t.Fatalf("вернулся чужой профиль: telegramId=%v, запрашивали %d", found[0].TelegramID, tgA)
	}
}
