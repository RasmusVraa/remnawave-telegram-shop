package service

// Синтетические отображаемые имена для демо-ленты (имитация реальных юзеров Telegram).
var fortuneFakeTickerNames = []string{
	// Обычные имена (разные вариации)
	"Alexey", "Dmitrich", "Sanya322", "Mariya_Ivanova", "Ivanich", "Vanya05",
	"ElenaV", "Tatyana_K", "Andryuha", "Sergey_Viktorovich", "Natasha", "Olegos",
	"Dimon_Chik", "Katerina", "Artemka", "Maximuz", "Yulya_Yulya", "Nikita2010",

	// Геймерские и сленговые
	"ShadowFiend", "Pudge_Fan", "CyberCote", "Killer777", "NoobMaster", "Zxcursed",
	"LegitPlayer", "SkillIssue", "FragMachine", "GamerGirl", "Pro_Gamer", "Axe",

	// Абстрактные и странные (как у многих в ТГ)
	"Solitude", "Silence", "Dark_Knight", "Wild_Soul", "Red_Dragon", "Blue_Wave",
	"Moonlight", "SunDay", "Forest_Guy", "Wind_Whisper", "Night_Owl", "Deep_Sea",

	// Лаконичные и короткие
	"v_v", "m_r", "z_x", "k_l", "q_p", "abc", "xyz", "nn", "user_1", "id_0",
	"lucky", "best", "top", "win", "ace", "sky", "fire", "ice", "bold", "fast",

	// Крипто- и тех-тематика (но не слишком сложная)
	"Crypto_King", "Eth_Holder", "Bitcoin_Fan", "Nft_Art", "Dev_Ops", "Admin",
	"Rich_Boi", "Money_Maker", "Stock_Master", "Web3_User", "Ton_Holder", "HODL",

	// Женские "инста" стили
	"Sweetie", "Princess", "Your_Dream", "Lady_In_Red", "Krasotka", "Mila",
	"Coffee_Lover", "Travel_Girl", "Yoga_Fan", "Happy_Mom", "Flower_Child",

	// На кириллице (Telegram это позволяет в Display Name)
	"Александр", "Мария", "Димон", "Света", "Игорь_ОК", "Просто_Макс",
	"Кот_Борис", "Аноним", "Удача", "Победа", "Мир", "Солнце",

	// С цифрами и спецсимволами
	"Agent_007", "King_99", "Boss_24", "User_2024", "Lucky_777", "X_Master_X",
	"V_I_P", "Star_1", "Mega_User", "Pro_User", "The_Best_One", "Top_Dog",

	// Рандомные и фановые
	"Panda_Express", "Pizza_Lover", "Beer_Fan", "Cat_Mom", "Dog_Dad", "Lazy_Boy",
	"Sleepy_Head", "Big_Boss", "Small_Fry", "Captain_Obvious", "No_Name", "Guest",
}

// Вращение типов «дней» для синтетических записей ленты (без XP/скидок).
var fortuneFakeTickerDayTypes = []string{
	"days_7", "days_3", "days_15", "days_5", "days_30", "days_3", "days_7", "days_5",
}

// Небольшая доля «не дней» только если лента целиком синтетическая (пустая БД).
var fortuneFakeTickerOtherTypes = []string{"discount_3", "xp", "micro", "discount_5"}
