package handler

const (
	CallbackBuy = "buy"

	// BroadcastInlineQuery — суффикс callback у кнопок под постом рассылки: обработчики не трогают исходное сообщение, шлют новое.
	BroadcastInlineQuery = "?bc=1"
	CallbackSell              = "sell"
	CallbackStart             = "start"
	CallbackConnect           = "connect"
	CallbackPayment           = "payment"
	CallbackTrial             = "trial"
	CallbackActivateTrial     = "activate_trial"
	CallbackReferral          = "referral"
	CallbackReferralList      = "referral_list"
	CallbackLoyaltyRoot       = "loyalty_root"
	CallbackManageDevices     = "manage_devices"
	CallbackDevices           = "devices"
	CallbackDeleteDevice      = "delete_device_"
	CallbackAddDevice         = "add_device"
	CallbackAddDeviceConfirm  = "add_device_confirm"
	CallbackAddDeviceApply    = "add_device_apply"
	CallbackAddDevicePayment  = "add_device_payment"
	CallbackPurchaseHistory   = "purchase_history"
	CallbackBroadcastConfirm  = "broadcast_confirm"
	CallbackBroadcastCancel   = "broadcast_cancel"
	CallbackBroadcastAll           = "broadcast_all"
	CallbackBroadcastActive        = "broadcast_active"
	CallbackBroadcastInactive      = "broadcast_inactive"
	CallbackBroadcastActivePaid    = "bc_aud_ap"
	CallbackBroadcastActiveTrial   = "bc_aud_at"
	CallbackBroadcastActiveAllSeg  = "bc_aud_aa"
	CallbackBroadcastInactivePaid  = "bc_aud_ip"
	CallbackBroadcastInactiveTrial = "bc_aud_it"
	CallbackBroadcastInactiveAllSeg = "bc_aud_ia"
	CallbackBroadcastBackAudience  = "bc_aud_b"
	CallbackBroadcastBackAdmin     = "broadcast_back_admin"
	// Режим tariffs: выбор тарифа для сегмента «платники» (префикс + id тарифа).
	CallbackBroadcastPaidTariffActivePrefix   = "bc_pt_a_"
	CallbackBroadcastPaidTariffInactivePrefix = "bc_pt_i_"
	CallbackBroadcastPaidTariffActiveAll      = "bc_pt_aa"
	CallbackBroadcastPaidTariffInactiveAll      = "bc_pt_ia"
	CallbackBroadcastPaidTariffBackActiveSeg    = "bc_pt_ba"
	CallbackBroadcastPaidTariffBackInactiveSeg = "bc_pt_bi"
	CallbackBroadcastToggleMain = "bc_t_main"
	CallbackBroadcastTogglePromo = "bc_t_prm"
	CallbackBroadcastToggleVPN   = "bc_t_vpn"
	CallbackBroadcastToggleBuy   = "bc_t_buy"
	CallbackBroadcastToggleShare = "bc_t_ref"
	CallbackBroadcastButtonsNext = "bc_next"

	CallbackAdminPanel   = "admin_panel"
	CallbackAdminBroadcast = "admin_bc"
	CallbackAdminSync    = "admin_sync"
	CallbackAdminPromo   = "admin_promo"
	CallbackAdminTariffs = "admin_tariffs"

	// Админ: пользователи и подписки (Bedolaga-style; короткие callback).
	CallbackAdminUsersSubmenu      = "au_sm"
	CallbackAdminUsersRoot         = "au_u"
	CallbackAdminUsersSearch       = "au_sh"
	CallbackAdminUsersStatsSection = "au_sec"
	CallbackAdminUsersInactiveMenu = "au_in"

	CallbackAdminUsersListAllPrefix      = "aula"
	CallbackAdminUsersListInactivePrefix = "auli"
	// Выбор страницы списка клиентов: ulkj — перейти на стр.; ulkp — сетка (chunk 3 цифры + return 3 цифры).
	CallbackAdminUsersListPagePickJumpPrefix = "ulkj"
	CallbackAdminUsersListPagePickOpenPrefix = "ulkp"

	CallbackAdminUserManagePrefix = "aum"
	// Карточка пользователя из уведомления об оплате в PAYMENTS_NOTIFY_CHAT_ID (новое сообщение; см. internal/payment/notify_payments_group.go).
	CallbackPaymentsNotifyUserOpenPrefix = "pnu"
	CallbackAdminUserSubscriptionPrefix = "aus"
	CallbackAdminUserReferralsPrefix    = "aur"
	CallbackAdminUserSpendPrefix        = "aut"
	CallbackAdminUserPaymentsPrefix     = "aup"

	CallbackAdminUserMsgHintPrefix = "aug"

	CallbackAdminUserExtendPrefix = "axe"

	// Сброс трафика: atq = подтверждение, atc = выполнить (префикс + id клиента).
	CallbackAdminUserResetTrafficAskPrefix    = "atq"
	CallbackAdminUserResetTrafficConfirmPrefix = "atc"

	CallbackAdminUserHwPresetMenuPrefix = "ahm"
	CallbackAdminUserHwPresetSetPrefix  = "ahp"

	// Календарь срока подписки: acl открыть, acn месяц, acp выбрать день, acb пустая ячейка.
	CallbackAdminUserCalOpenPrefix  = "acl"
	CallbackAdminUserCalNavPrefix   = "acn"
	CallbackAdminUserCalPickPrefix  = "acp"
	CallbackAdminUserCalBlankPrefix = "acb"
	CallbackAdminUserCalManualPrefix = "acm"

	// Расширенные настройки панели: apm меню, asq сквад, ats/att стратегия, atl/atg лимит трафика, arq/arc отключить, ueq/uec включить, ayq/ayc удалить.
	CallbackAdminUserPanelMenuPrefix    = "apm"
	CallbackAdminUserSquadMenuPrefix    = "asl"
	CallbackAdminUserSquadPickPrefix    = "asq"
	CallbackAdminUserStrategyMenuPrefix = "ats"
	CallbackAdminUserStrategySetPrefix  = "att"
	CallbackAdminUserTrafficMenuPrefix  = "atl"
	CallbackAdminUserTrafficSetPrefix   = "atg"
	CallbackAdminUserTrafficCustomPrefix  = "atx"
	CallbackAdminUserDisableAskPrefix    = "arq"
	CallbackAdminUserDisableConfirmPrefix = "arc"
	CallbackAdminUserEnableAskPrefix     = "ueq"
	CallbackAdminUserEnableConfirmPrefix = "uec"
	CallbackAdminUserDeleteAskPrefix      = "ayq"
	CallbackAdminUserDeleteConfirmPrefix  = "ayc"

	CallbackAdminUserDevicesPrefix = "adv"
	CallbackAdminUserDevDelPrefix  = "adx"

	// Доп. HWID в БД (ehm/ehp + id), тариф (utm меню, utp + id_tariffId), описание в панели (uds запрос текста, udc очистка).
	CallbackAdminUserExtraHwidDecPrefix = "ehm"
	CallbackAdminUserExtraHwidIncPrefix = "ehp"
	CallbackAdminUserTariffMenuPrefix   = "utm"
	CallbackAdminUserTariffPickPrefix   = "utp"
	CallbackAdminUserDescAskPrefix      = "uds"
	CallbackAdminUserDescClearPrefix    = "udc"

	CallbackAdminSubsRoot       = "sbr"
	CallbackAdminSubsListPrefix = "sbl"
	CallbackAdminSubsExpiring   = "sbe"
	// Пагинация списка «скоро истекают» (sbe только открывает 0-ю страницу).
	CallbackAdminSubsExpiringListPrefix = "sbx"
	CallbackAdminSubsStatsJump  = "sbs"

	CallbackAdminRefRoot = "arf"

	// Админ: лояльность (префиксы ly_*)
	CallbackAdminLoyaltyRoot       = "ly_r"
	CallbackAdminLoyaltyLevels     = "ly_l"
	CallbackAdminLoyaltyCard       = "ly_v" // ly_v?i=id
	CallbackAdminLoyaltyNew        = "ly_n"
	CallbackAdminLoyaltyDelAsk     = "ly_del" // не "ly_d" — префикс конфликтует с ly_dn
	CallbackAdminLoyaltyDelYes     = "ly_y"
	CallbackAdminLoyaltyEditXP     = "ly_x"
	CallbackAdminLoyaltyEditPct    = "ly_p"
	CallbackAdminLoyaltyRecalcAsk  = "ly_rc"
	CallbackAdminLoyaltyRecalcRun  = "ly_rr"
	CallbackAdminLoyaltyRules      = "ly_rules"
	CallbackAdminLoyaltyStats      = "ly_st"
	CallbackAdminLoyaltyEditDn     = "ly_dn"

	CallbackAdminStatsRoot    = "as_r"
	CallbackAdminStatsUsers   = "as_u"
	CallbackAdminStatsSubs    = "as_s"
	CallbackAdminStatsRevenue = "as_v"
	CallbackAdminStatsRef     = "as_f"
	CallbackAdminStatsSummary = "as_m"
	CallbackAdminStatsFortune = "as_w"

	CallbackAdminInfraRoot   = "ib_r"
	CallbackAdminInfraNodes  = "ib_n"
	CallbackAdminInfraNotify = "ib_u"
	CallbackAdminInfraHist   = "ib_h"
	CallbackAdminInfraProv   = "ib_p"
	CallbackAdminInfraToggle = "ibt"
	CallbackTariffNew    = "tf_new"
	// Префиксы callback: tf_v?, tf_t?, tf_d?, tf_y?, tf_s?, tf_q?, tf_sc?, tf_sa?, tf_nm?, tf_tt?, tf_td?, tf_tl?, tf_ep?, tf_ds?, tf_ca?, tf_wc?

	CallbackPromoRoot     = "promo_root"
	CallbackPromoList     = "promo_list"
	CallbackPromoNew      = "promo_new"
	CallbackPromoStatsAll = "promo_stats_all"
	CallbackPromoCard     = "promo_card"
	CallbackPromoNewType  = "promo_ntype"
	CallbackPromoDiscKind       = "promo_dk"
	CallbackPromoSubDaysScope   = "promo_sds"
	CallbackPromoDel      = "promo_del"
	CallbackPromoDelYes   = "promo_yes"
	CallbackPromoToggle   = "promo_toggle"
	CallbackPromoFirstPur = "promo_fp"
	CallbackPromoStat      = "promo_stat"
	CallbackPromoEdit          = "promo_edit"
	CallbackPromoEditValid     = "promo_ev"
	CallbackPromoEditMax       = "promo_em"
	CallbackPromoEditSubDays   = "promo_esd"
	CallbackPromoEditTrialDays = "promo_etd"
	CallbackPromoEditSubsTariff    = "promo_xts"
	CallbackPromoEditSubsTariffSet = "promo_xta"
	CallbackPromoEditDiscPay       = "promo_edp"
	CallbackEnterPromo         = "enter_promo"

	CallbackLegalAccept  = "legal_accept"
	CallbackLegalDecline = "legal_decline"
)
