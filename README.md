# MISALIGN Discord Bot (Go) — 3~4인 베타 MVP

이 프로젝트는 **MISALIGN 룰을 실제 개발 전에 검증**하기 위한 Discord 기반 진행/사회자 봇입니다.

## MVP 목표
- 3~4인 플레이에 최적화
- 6라운드 자동 진행
- 8개 미니게임 중 6개 랜덤 선택(중복 없음)
- 서버(봇) 기준 점수 계산 고정
- 능력 1인 1개, 게임 전체 1회 사용
- 음성/심리전 중심 플레이를 보조하는 진행자 역할

## 현재 구현 범위
- 로비 생성/참가/시작
- 라운드 진행 및 선택 수집(DM)
- 미니게임 점수 계산
- 능력 8종 기본 구현
- 점수판/상태 출력
- 호스트 강제 마감

## 명령어
- `!help`
- `!create`
- `!join`
- `!start`
- `!status`
- `!next` (호스트)
- `!choose <값>` (DM 권장)
- `!ability <ability> [targetUserID] [value]`

능력 키워드:
`intervention`, `blackout`, `assimilation`, `lock`, `mask`, `share`, `scan`, `metaview`

## 실행
```bash
go mod tidy
go run .
```

필수 환경변수:
```bash
export DISCORD_BOT_TOKEN="YOUR_BOT_TOKEN"
```

## MVP 운영 팁
- 실제 게임은 음성 채널에서 진행하고, 선택 입력만 DM으로 처리하세요.
- 라운드 시간이 길다면 로컬 테스트 시 `!next`로 즉시 마감하세요.
- 3~4인 기준으로 설계되어 있습니다.

## 설계 메모 (MVP 한정)
- 점수 진실성: 계산은 항상 봇에서 처리
- 정보 교란: blackout/mask/share/scan/metaview로 심리전 강화
- 공적 증명 불가: DM 기반 정보 전달 중심

