# DEV 가이드 — Discord Bot 등록 및 실행

## 1) Discord Developer Portal에서 봇 생성
1. https://discord.com/developers/applications 접속
2. `New Application` 클릭 후 이름 입력
3. 좌측 `Bot` 메뉴 이동
4. `Add Bot` 클릭
5. `Reset Token` 후 토큰 발급 (외부 노출 금지)

## 2) Privileged Gateway Intents 설정
`Bot` 탭에서 아래 Intent를 활성화하세요.
- `MESSAGE CONTENT INTENT`

이 프로젝트는 prefix 명령(`!create` 등)을 메시지 내용으로 파싱하므로 필수입니다.

## 3) 봇 초대 URL 생성
1. 좌측 `OAuth2` → `URL Generator`
2. Scopes:
   - `bot`
3. Bot Permissions (최소 권한 예시):
   - View Channels
   - Send Messages
   - Read Message History
4. 생성된 URL로 서버에 봇 초대

## 4) 로컬 실행
```bash
cd /workspace/MISALIGN
go mod tidy
export DISCORD_BOT_TOKEN="YOUR_BOT_TOKEN"
go run .
```

정상 실행 시 로그:
- `MISALIGN bot is running`

## 5) 기본 플레이 플로우
1. 서버 텍스트 채널에서 `!create`
2. 플레이어 3~4명 `!join`
3. 호스트 `!start`
4. 라운드마다 각자 DM으로 `!choose <값>` 입력
5. 능력 사용 시 `!ability ...`
6. 필요 시 호스트 `!next`

## 6) 운영 시 권장 세팅
- 실제 플레이는 음성 채널에서 진행
- 봇은 텍스트 채널 1개 + DM 수집 역할
- 테스트 시 3분 타이머 대신 `!next` 적극 활용

## 7) 문제 해결
- 봇이 명령을 무시함
  - 토큰 확인
  - Message Content Intent 활성화 여부 확인
  - 봇 초대 권한 확인
- DM 전송 실패
  - 사용자가 서버 DM 허용/친구외 DM 허용 옵션 확인
- `go run .` 실패
  - Go 버전 확인(1.22+ 권장)
  - `go mod tidy` 재실행

