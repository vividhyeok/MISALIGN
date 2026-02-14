package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	minPlayers  = 3
	maxPlayers  = 4
	totalRounds = 6
)

type MiniGame string

type Ability string

const (
	MGTraitorSplit  MiniGame = "배신 분배"
	MGMatch123      MiniGame = "1-2-3 일치"
	MGSecretAuction MiniGame = "비밀 경매"
	MGCircularTrack MiniGame = "원형 트랙"
	MGRace          MiniGame = "경마"
	MGTimingButton  MiniGame = "타이밍 버튼"
	MGOX            MiniGame = "OX 심리"
	MGNunchi        MiniGame = "눈치 도전"
)

const (
	Intervention Ability = "intervention"
	Blackout     Ability = "blackout"
	Assimilation Ability = "assimilation"
	Lock         Ability = "lock"
	Mask         Ability = "mask"
	Share        Ability = "share"
	Scan         Ability = "scan"
	MetaView     Ability = "metaview"
)

var allMiniGames = []MiniGame{MGTraitorSplit, MGMatch123, MGSecretAuction, MGCircularTrack, MGRace, MGTimingButton, MGOX, MGNunchi}
var allAbilities = []Ability{Intervention, Blackout, Assimilation, Lock, Mask, Share, Scan, MetaView}

type PlayerState struct {
	ID             string
	Name           string
	Score          int
	Ability        Ability
	AbilityUsed    bool
	Choices        map[int]string
	LastRaceAction string
	TrackPos       int
	UsedAbilityAt  int
}

type AbilityEffect struct {
	Interventions map[string]string
	Blackout      bool
	Locks         map[string]bool
	Assimilation  map[string]string
	MaskTargets   map[string]bool
	PrivateNotes  []string
}

type GameState struct {
	HostID           string
	ChannelID        string
	GuildID          string
	Status           string
	Round            int
	Players          map[string]*PlayerState
	PlayerOrder      []string
	Minigames        []MiniGame
	CurrentGame      MiniGame
	ChoicesReceived  map[string]bool
	RoundStartedAt   time.Time
	AbilityEffects   AbilityEffect
	RoundLogs        map[string][]string
	RaceHorsePos     [3]int
	RaceHorseLengths [3]int
	NunchiResolved   bool
	mu               sync.Mutex
}

type Bot struct {
	dg    *discordgo.Session
	games map[string]*GameState
	mu    sync.Mutex
}

func main() {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_BOT_TOKEN is required")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}

	bot := &Bot{dg: dg, games: map[string]*GameState{}}
	dg.AddHandler(bot.onMessageCreate)
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent

	if err := dg.Open(); err != nil {
		log.Fatal(err)
	}
	defer dg.Close()

	log.Println("MISALIGN bot is running")
	select {}
}

func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot || !strings.HasPrefix(strings.TrimSpace(m.Content), "!") {
		return
	}
	args := strings.Fields(strings.TrimSpace(m.Content))
	cmd := strings.ToLower(strings.TrimPrefix(args[0], "!"))

	switch cmd {
	case "help":
		b.sendHelp(m.ChannelID)
	case "create":
		b.createGame(m)
	case "join":
		b.joinGame(m)
	case "start":
		b.startGame(m)
	case "status":
		b.statusGame(m.ChannelID)
	case "choose":
		b.choose(m, args)
	case "ability":
		b.useAbility(m, args)
	case "next":
		b.forceResolveRound(m)
	default:
		_, _ = s.ChannelMessageSend(m.ChannelID, "알 수 없는 명령입니다. `!help`를 확인하세요.")
	}
}

func (b *Bot) sendHelp(ch string) {
	msg := "**MISALIGN MVP 명령어**\n" +
		"- `!create` : 현재 채널에서 게임 생성\n" +
		"- `!join` : 게임 참가 (3~4인)\n" +
		"- `!start` : 호스트가 게임 시작\n" +
		"- `!choose <선택값>` : DM으로 라운드 선택 제출\n" +
		"- `!ability <능력> [대상ID] [값]` : 능력 사용\n" +
		"- `!status` : 현재 점수/라운드 확인\n" +
		"- `!next` : (호스트) 현재 라운드 즉시 마감\n\n" +
		"능력 키워드: intervention, blackout, assimilation, lock, mask, share, scan, metaview"
	_, _ = b.dg.ChannelMessageSend(ch, msg)
}

func (b *Bot) createGame(m *discordgo.MessageCreate) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.games[m.ChannelID]; ok {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "이미 게임이 존재합니다.")
		return
	}
	gs := &GameState{
		HostID:           m.Author.ID,
		ChannelID:        m.ChannelID,
		GuildID:          m.GuildID,
		Status:           "lobby",
		Players:          map[string]*PlayerState{},
		RoundLogs:        map[string][]string{},
		RaceHorseLengths: [3]int{9, 8, 7},
	}
	b.games[m.ChannelID] = gs
	_, _ = b.dg.ChannelMessageSend(m.ChannelID, "게임이 생성되었습니다. `!join`으로 3~4인이 참가한 뒤 `!start` 하세요.")
}

func (b *Bot) getGame(channelID string) (*GameState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	gs, ok := b.games[channelID]
	if !ok {
		return nil, errors.New("게임이 없습니다. `!create`를 먼저 실행하세요")
	}
	return gs, nil
}

func (b *Bot) joinGame(m *discordgo.MessageCreate) {
	gs, err := b.getGame(m.ChannelID)
	if err != nil {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.Status != "lobby" {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "이미 시작된 게임입니다.")
		return
	}
	if len(gs.Players) >= maxPlayers {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "MVP는 4인까지 지원합니다.")
		return
	}
	if _, ok := gs.Players[m.Author.ID]; ok {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "이미 참가했습니다.")
		return
	}
	gs.Players[m.Author.ID] = &PlayerState{ID: m.Author.ID, Name: m.Author.Username, Choices: map[int]string{}, UsedAbilityAt: -1}
	gs.PlayerOrder = append(gs.PlayerOrder, m.Author.ID)
	_, _ = b.dg.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s 참가 완료 (%d/%d)", m.Author.Mention(), len(gs.Players), maxPlayers))
}

func (b *Bot) startGame(m *discordgo.MessageCreate) {
	gs, err := b.getGame(m.ChannelID)
	if err != nil {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}
	gs.mu.Lock()
	if gs.HostID != m.Author.ID {
		gs.mu.Unlock()
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "호스트만 시작할 수 있습니다.")
		return
	}
	if len(gs.Players) < minPlayers {
		gs.mu.Unlock()
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "최소 3인이 필요합니다.")
		return
	}
	if gs.Status != "lobby" {
		gs.mu.Unlock()
		return
	}
	gs.Status = "running"
	gs.Round = 0
	gs.Minigames = pick6MiniGames()
	assignAbilities(gs)
	gs.mu.Unlock()

	_, _ = b.dg.ChannelMessageSend(m.ChannelID, "게임 시작! 능력은 DM으로 안내됩니다. 라운드마다 DM으로 `!choose`를 입력하세요.")
	for _, pid := range gs.PlayerOrder {
		pl := gs.Players[pid]
		ch, _ := b.dg.UserChannelCreate(pid)
		_, _ = b.dg.ChannelMessageSend(ch.ID, fmt.Sprintf("당신의 능력: **%s** (게임 전체 1회)", pl.Ability))
	}
	b.startNextRound(gs)
}

func pick6MiniGames() []MiniGame {
	p := append([]MiniGame{}, allMiniGames...)
	rand.Shuffle(len(p), func(i, j int) { p[i], p[j] = p[j], p[i] })
	return p[:6]
}

func assignAbilities(gs *GameState) {
	ab := append([]Ability{}, allAbilities...)
	rand.Shuffle(len(ab), func(i, j int) { ab[i], ab[j] = ab[j], ab[i] })
	for i, pid := range gs.PlayerOrder {
		gs.Players[pid].Ability = ab[i]
	}
}

func (b *Bot) startNextRound(gs *GameState) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.Round >= totalRounds {
		b.finishGame(gs)
		return
	}
	gs.Round++
	gs.CurrentGame = gs.Minigames[gs.Round-1]
	gs.ChoicesReceived = map[string]bool{}
	gs.RoundStartedAt = time.Now()
	gs.AbilityEffects = AbilityEffect{Interventions: map[string]string{}, Locks: map[string]bool{}, Assimilation: map[string]string{}, MaskTargets: map[string]bool{}}
	gs.NunchiResolved = false

	announce := fmt.Sprintf("## Round %d/%d\n미니게임: **%s**\n30초 설명 후 3분 입력 시간. 각자 DM으로 `!choose <값>` 입력하세요.", gs.Round, totalRounds, gs.CurrentGame)
	_, _ = b.dg.ChannelMessageSend(gs.ChannelID, announce)
	for _, pid := range gs.PlayerOrder {
		ch, _ := b.dg.UserChannelCreate(pid)
		_, _ = b.dg.ChannelMessageSend(ch.ID, fmt.Sprintf("Round %d - %s 선택지를 제출하세요: %s", gs.Round, gs.CurrentGame, choiceGuide(gs.CurrentGame)))
	}

	go func(round int) {
		time.Sleep(3 * time.Minute)
		gs.mu.Lock()
		if gs.Round != round || gs.Status != "running" {
			gs.mu.Unlock()
			return
		}
		gs.mu.Unlock()
		b.resolveRound(gs, false)
	}(gs.Round)
}

func choiceGuide(mg MiniGame) string {
	switch mg {
	case MGTraitorSplit:
		return "협력 또는 배신"
	case MGMatch123:
		return "1 또는 2 또는 3"
	case MGSecretAuction:
		return "0~5"
	case MGCircularTrack:
		return "0~3"
	case MGRace:
		return "horse1 boost / horse2 sabotage 형태"
	case MGTimingButton:
		return "click 또는 pass"
	case MGOX:
		return "5문항 예: OXOXO"
	case MGNunchi:
		return "예 또는 아니오"
	}
	return ""
}

func (b *Bot) choose(m *discordgo.MessageCreate, args []string) {
	if len(args) < 2 {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "사용법: !choose <값>")
		return
	}
	choice := strings.Join(args[1:], " ")
	// find running game containing player
	var gs *GameState
	b.mu.Lock()
	for _, g := range b.games {
		if g.Status == "running" {
			if _, ok := g.Players[m.Author.ID]; ok {
				gs = g
				break
			}
		}
	}
	b.mu.Unlock()
	if gs == nil {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "진행중인 게임 참가자가 아닙니다.")
		return
	}

	gs.mu.Lock()
	pl := gs.Players[m.Author.ID]
	pl.Choices[gs.Round] = choice
	gs.ChoicesReceived[m.Author.ID] = true
	done := len(gs.ChoicesReceived) == len(gs.Players)
	round := gs.Round
	gs.mu.Unlock()
	_, _ = b.dg.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Round %d 선택 접수 완료", round))
	if done {
		b.resolveRound(gs, false)
	}
}

func (b *Bot) resolveRound(gs *GameState, forced bool) {
	gs.mu.Lock()
	if gs.Status != "running" {
		gs.mu.Unlock()
		return
	}
	round := gs.Round
	mg := gs.CurrentGame
	choices := map[string]string{}
	for pid, pl := range gs.Players {
		ch := pl.Choices[round]
		if iv, ok := gs.AbilityEffects.Interventions[pid]; ok {
			ch = iv
		}
		choices[pid] = ch
	}
	deltas := evaluateRound(gs, mg, choices)

	for pid, tpid := range gs.AbilityEffects.Assimilation {
		deltas[pid] = deltas[tpid]
	}
	for pid, d := range deltas {
		gs.Players[pid].Score += d
		gs.RoundLogs[pid] = append(gs.RoundLogs[pid], fmt.Sprintf("R%d %s choice=%s delta=%d total=%d", round, mg, choices[pid], d, gs.Players[pid].Score))
	}

	resultLines := []string{fmt.Sprintf("### Round %d 결과 - %s", round, mg)}
	if gs.AbilityEffects.Blackout {
		resultLines = append(resultLines, "Blackout 발동: 점수판은 비공개입니다.")
	} else {
		type row struct {
			n string
			s int
		}
		rows := []row{}
		for _, pid := range gs.PlayerOrder {
			rows = append(rows, row{gs.Players[pid].Name, gs.Players[pid].Score})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].s > rows[j].s })
		for _, r := range rows {
			resultLines = append(resultLines, fmt.Sprintf("- %s: %d", r.n, r.s))
		}
	}
	if forced {
		resultLines = append(resultLines, "(호스트 강제 마감)")
	}
	_, _ = b.dg.ChannelMessageSend(gs.ChannelID, strings.Join(resultLines, "\n"))
	gs.mu.Unlock()

	b.startNextRound(gs)
}

func evaluateRound(gs *GameState, mg MiniGame, choices map[string]string) map[string]int {
	out := map[string]int{}
	for pid := range gs.Players {
		out[pid] = 0
	}
	switch mg {
	case MGTraitorSplit:
		traitors := []string{}
		for pid, c := range choices {
			if strings.Contains(c, "배신") {
				traitors = append(traitors, pid)
			}
		}
		if len(traitors) == 0 {
			for pid := range out {
				out[pid] = 0
			}
			return out
		}
		gain := int(math.Floor(6.0 / float64(len(traitors))))
		for _, pid := range traitors {
			out[pid] = gain
		}
		for pid := range choices {
			if !contains(traitors, pid) {
				out[pid] = -3
			}
		}
	case MGMatch123:
		count := map[string]int{}
		for _, c := range choices {
			count[c]++
		}
		if len(count) == len(choices) {
			for pid := range out {
				out[pid] = -1
			}
			return out
		}
		for n, c := range count {
			if c == 2 {
				for pid, v := range choices {
					if v == n {
						out[pid] = 4
					}
				}
			}
			if c >= 3 {
				for pid, v := range choices {
					if v == n {
						out[pid] = -2
					}
				}
			}
		}
	case MGSecretAuction:
		levels := map[int][]string{}
		for pid, c := range choices {
			n, _ := strconv.Atoi(strings.TrimSpace(c))
			levels[n] = append(levels[n], pid)
		}
		for v := 5; v >= 0; v-- {
			if len(levels[v]) == 1 {
				out[levels[v][0]] = 6
				break
			}
		}
	case MGCircularTrack:
		counts := map[string]int{}
		for _, c := range choices {
			counts[c]++
		}
		var target string
		best := -1
		for n, c := range counts {
			if c > best {
				best = c
				target = n
			}
		}
		mv, _ := strconv.Atoi(target)
		for pid, c := range choices {
			if c == target {
				pl := gs.Players[pid]
				pl.TrackPos += mv
				if pl.TrackPos == 9 {
					out[pid] = 6
					pl.TrackPos = 0
				}
				if pl.TrackPos > 9 {
					pl.TrackPos = 0
				}
			}
		}
	case MGRace:
		moves := [3]int{}
		for pid, c := range choices {
			parts := strings.Fields(strings.ToLower(c))
			if len(parts) != 2 {
				continue
			}
			h, act := parts[0], parts[1]
			hi := strings.TrimPrefix(h, "horse")
			idx, _ := strconv.Atoi(hi)
			if idx < 1 || idx > 3 {
				continue
			}
			if act == "sabotage" && gs.Players[pid].LastRaceAction == "sabotage" {
				continue
			}
			if act == "boost" {
				moves[idx-1]++
			}
			if act == "sabotage" {
				moves[idx-1]--
			}
			gs.Players[pid].LastRaceAction = act
		}
		for i := 0; i < 3; i++ {
			gs.RaceHorsePos[i] += moves[i]
			if gs.RaceHorsePos[i] < 0 {
				gs.RaceHorsePos[i] = 0
			}
		}
		rank := []int{0, 1, 2}
		sort.Slice(rank, func(i, j int) bool { return gs.RaceHorsePos[rank[i]] > gs.RaceHorsePos[rank[j]] })
		for pid, c := range choices {
			parts := strings.Fields(strings.ToLower(c))
			if len(parts) == 0 {
				continue
			}
			idx, _ := strconv.Atoi(strings.TrimPrefix(parts[0], "horse"))
			idx--
			if idx == rank[0] {
				out[pid] = 5
			} else if idx == rank[1] {
				out[pid] = 3
			} else {
				out[pid] = -3
			}
		}
	case MGTimingButton:
		clickers := []string{}
		for pid, c := range choices {
			if strings.Contains(strings.ToLower(c), "click") {
				clickers = append(clickers, pid)
			}
		}
		n := len(clickers)
		if n == 0 {
			return out
		}
		score := n
		if n%2 == 0 {
			score = -n
		}
		for _, pid := range clickers {
			out[pid] = score
		}
	case MGOX:
		for pid := range choices {
			out[pid] = 0
		}
	case MGNunchi:
		yes := []string{}
		for pid, c := range choices {
			if strings.Contains(c, "예") || strings.EqualFold(c, "yes") {
				yes = append(yes, pid)
			}
		}
		if len(yes) == 1 {
			out[yes[0]] = 5
			gs.NunchiResolved = true
		}
		if len(yes) >= 2 {
			for _, pid := range yes {
				out[pid] = -3
			}
			gs.NunchiResolved = true
		}
	}
	return out
}

func (b *Bot) useAbility(m *discordgo.MessageCreate, args []string) {
	if len(args) < 2 {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "사용법: !ability <ability> [targetUserID] [value]")
		return
	}
	ab := Ability(strings.ToLower(args[1]))
	var gs *GameState
	b.mu.Lock()
	for _, g := range b.games {
		if g.Status == "running" {
			if _, ok := g.Players[m.Author.ID]; ok {
				gs = g
				break
			}
		}
	}
	b.mu.Unlock()
	if gs == nil {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "진행 중 게임이 없습니다.")
		return
	}

	gs.mu.Lock()
	defer gs.mu.Unlock()
	pl := gs.Players[m.Author.ID]
	if pl.Ability != ab {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "본인 능력과 다릅니다.")
		return
	}
	if pl.AbilityUsed {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "이미 사용한 능력입니다.")
		return
	}
	if gs.AbilityEffects.Locks[m.Author.ID] {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "Lock으로 이번 턴 능력 사용이 봉쇄되었습니다.")
		return
	}

	target := ""
	if len(args) >= 3 {
		target = strings.Trim(args[2], "<@!>")
	}
	value := ""
	if len(args) >= 4 {
		value = strings.Join(args[3:], " ")
	}

	switch ab {
	case Intervention:
		if target == "" || value == "" {
			_, _ = b.dg.ChannelMessageSend(m.ChannelID, "intervention은 대상ID와 강제값이 필요합니다")
			return
		}
		gs.AbilityEffects.Interventions[target] = value
	case Blackout:
		gs.AbilityEffects.Blackout = true
	case Assimilation:
		if target == "" {
			_, _ = b.dg.ChannelMessageSend(m.ChannelID, "assimilation은 대상ID가 필요합니다")
			return
		}
		gs.AbilityEffects.Assimilation[m.Author.ID] = target
	case Lock:
		if target == "" {
			_, _ = b.dg.ChannelMessageSend(m.ChannelID, "lock은 대상ID가 필요합니다")
			return
		}
		gs.AbilityEffects.Locks[target] = true
		used := gs.Players[target].AbilityUsed
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Lock 정보: 대상의 과거 능력 사용 여부 = %v", used))
	case Mask:
		gs.AbilityEffects.MaskTargets[m.Author.ID] = true
	case Share:
		if target == "" {
			_, _ = b.dg.ChannelMessageSend(m.ChannelID, "share는 대상ID가 필요합니다")
			return
		}
		ch, _ := b.dg.UserChannelCreate(target)
		_, _ = b.dg.ChannelMessageSend(ch.ID, fmt.Sprintf("[Share] %s의 라운드 로그:\n%s", pl.Name, strings.Join(gs.RoundLogs[m.Author.ID], "\n")))
	case Scan:
		if target == "" {
			_, _ = b.dg.ChannelMessageSend(m.ChannelID, "scan은 대상ID가 필요합니다")
			return
		}
		has := !gs.Players[target].AbilityUsed
		ch, _ := b.dg.UserChannelCreate(m.Author.ID)
		_, _ = b.dg.ChannelMessageSend(ch.ID, fmt.Sprintf("Scan 결과: 대상 능력 보유(미사용) 여부 = %v", has))
	case MetaView:
		set := map[Ability]bool{}
		for _, p := range gs.Players {
			set[p.Ability] = true
		}
		kinds := []string{}
		for k := range set {
			kinds = append(kinds, string(k))
		}
		sort.Strings(kinds)
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, fmt.Sprintf("MetaView: 현재 게임에 존재하는 능력 종류 = %s / 능력 선택자 수 = %d", strings.Join(kinds, ", "), len(gs.Players)))
	default:
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "알 수 없는 능력")
		return
	}
	pl.AbilityUsed = true
	pl.UsedAbilityAt = gs.Round
	_, _ = b.dg.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s 능력 사용 완료", pl.Name))
}

func (b *Bot) forceResolveRound(m *discordgo.MessageCreate) {
	gs, err := b.getGame(m.ChannelID)
	if err != nil {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}
	if gs.HostID != m.Author.ID {
		_, _ = b.dg.ChannelMessageSend(m.ChannelID, "호스트만 사용 가능")
		return
	}
	b.resolveRound(gs, true)
}

func (b *Bot) statusGame(channelID string) {
	gs, err := b.getGame(channelID)
	if err != nil {
		_, _ = b.dg.ChannelMessageSend(channelID, err.Error())
		return
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	lines := []string{fmt.Sprintf("상태: %s / Round %d/%d", gs.Status, gs.Round, totalRounds)}
	for _, pid := range gs.PlayerOrder {
		p := gs.Players[pid]
		lines = append(lines, fmt.Sprintf("- %s: %d (ability:%s used:%v)", p.Name, p.Score, p.Ability, p.AbilityUsed))
	}
	_, _ = b.dg.ChannelMessageSend(channelID, strings.Join(lines, "\n"))
}

func (b *Bot) finishGame(gs *GameState) {
	type row struct {
		n string
		s int
	}
	rows := []row{}
	for _, pid := range gs.PlayerOrder {
		p := gs.Players[pid]
		rows = append(rows, row{p.Name, p.Score})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].s > rows[j].s })
	lines := []string{"## 게임 종료"}
	for i, r := range rows {
		lines = append(lines, fmt.Sprintf("%d위 %s (%d)", i+1, r.n, r.s))
	}
	_, _ = b.dg.ChannelMessageSend(gs.ChannelID, strings.Join(lines, "\n"))
	gs.Status = "finished"
}

func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}
