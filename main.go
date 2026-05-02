package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"image/color"
	_ "image/png"
	"math"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

//go:embed assets/gopher.png
var gopherPNG []byte

//go:embed assets/font.ttf
var fontTTF []byte

// 描画パラメータ
const (
	fontSize     = 24
	maxLineWidth = 350 // テキスト自動改行の最大ピクセル幅
	maxGopherPx  = 300 // Gopher画像の最大表示サイズ(px)

	bubblePadX    = 44  // 吹き出し左右の余白
	bubblePadY    = 28  // 吹き出し上下の余白
	bubbleRadius  = 15  // 吹き出し角丸の半径
	bubbleGap     = 25  // 吹き出しとGopherの間隔
	lineSpacing   = 4   // 行間の追加ピクセル
	strokeWidth   = 2   // 枠線の太さ
	minWindowSize = 300 // ウィンドウ最小サイズ(Metal描画エラー回避)
)

func main() {
	game, err := NewGame()
	if err != nil {
		panic(err)
	}

	ebiten.SetWindowSize(game.screenWidth, game.screenHeight)

	monitor := ebiten.Monitor()
	monitorWidth, monitorHeight := monitor.Size()
	ebiten.SetWindowPosition(monitorWidth-game.screenWidth, monitorHeight-game.screenHeight)
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)

	if err := ebiten.RunGameWithOptions(game, &ebiten.RunGameOptions{
		ScreenTransparent: true,
	}); err != nil {
		panic(err)
	}
}

// layout は事前に計算された描画レイアウト情報。
type layout struct {
	gopherX, gopherY float64
	gopherScale      float64
	bubbleX, bubbleY float32
	bubbleW, bubbleH float32
	lines            []string
	lineHeight       float64
}

// --- テキストユーティリティ ---

// wrapText は文字列を指定のピクセル幅で自動改行する。既存の改行(\n)は保持する。
func wrapText(msg string, face font.Face, maxWidth float64) string {
	var result []string
	for _, para := range strings.Split(msg, "\n") {
		if para == "" {
			result = append(result, "")
			continue
		}
		var line []rune
		for _, r := range para {
			candidate := append(line, r)
			if measureText(face, string(candidate)) > maxWidth && len(line) > 0 {
				result = append(result, string(line))
				line = []rune{r}
			} else {
				line = candidate
			}
		}
		if len(line) > 0 {
			result = append(result, string(line))
		}
	}
	return strings.Join(result, "\n")
}

// measureText はフォントでレンダリングした際のテキスト幅(px)を返す。
func measureText(face font.Face, str string) float64 {
	bounds, _ := font.BoundString(face, str)
	return float64(bounds.Max.X.Round() - bounds.Min.X.Round())
}

// maxTextWidth は複数行のうち最も幅の広い行のピクセル幅を返す。
func maxTextWidth(face font.Face, lines []string) float64 {
	var max float64
	for _, line := range lines {
		if w := measureText(face, line); w > max {
			max = w
		}
	}
	return max
}

// --- リソース読み込み ---

func loadGopherImage() (*ebiten.Image, error) {
	img, _, err := ebitenutil.NewImageFromReader(bytes.NewReader(gopherPNG))
	if err != nil {
		return nil, fmt.Errorf("new image: %w", err)
	}
	return img, nil
}

func loadFontFace() (font.Face, error) {
	tt, err := opentype.Parse(fontTTF)
	if err != nil {
		return nil, fmt.Errorf("parse font: %w", err)
	}
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("new font face: %w", err)
	}
	return face, nil
}

// --- レイアウト計算 ---

// calcGopherScale は画像サイズに応じたスケール係数を返す。
func calcGopherScale(img *ebiten.Image) float64 {
	w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	return math.Min(float64(maxGopherPx)/w, float64(maxGopherPx)/h)
}

// calcLayout は全要素のサイズ・配置を一括計算し、ウィンドウサイズも返す。
func calcLayout(img *ebiten.Image, face font.Face, message string) (layout, int, int) {
	// Gopherサイズ（固定基準）
	scale := calcGopherScale(img)
	gopherW := float64(img.Bounds().Dx()) * scale
	gopherH := float64(img.Bounds().Dy()) * scale

	// Gopherの固定位置（ウィンドウ右下に固定マージン）
	gopherMarginRight := 20.0
	gopherMarginBottom := 5.0

	// テキスト計測
	lines := strings.Split(message, "\n")
	lineH := float64(fontSize) + lineSpacing

	var bw, bh float64
	if message != "" {
		textW := maxTextWidth(face, lines)
		textH := float64(len(lines)) * lineH
		bw = textW + bubblePadX
		bh = textH + bubblePadY
	}

	// ウィンドウサイズ（Gopherの位置が変わらないようにGopher基準で計算）
	// メッセージがなくても吹き出し分のスペースを確保し、初回入力時の急激なリサイズを防ぐ
	minBubbleH := float64(fontSize+lineSpacing) + bubblePadY // 1行分の最小バブル高さ
	effectiveBH := math.Max(bh, minBubbleH)
	sw := int(math.Max(bw+80, gopherW+gopherMarginRight+20))
	sh := int(gopherH + gopherMarginBottom + bubbleGap + effectiveBH + 20)
	if sw < minWindowSize {
		sw = minWindowSize
	}
	if sh < minWindowSize {
		sh = minWindowSize
	}

	// Gopher配置（常にウィンドウ右下に固定）
	gopherX := float64(sw) - gopherW - gopherMarginRight
	gopherY := float64(sh) - gopherH - gopherMarginBottom

	// 吹き出し配置（Gopherの上に配置）
	bx32 := float32(float64(sw)/2) - float32(bw)/2
	by32 := float32(gopherY - bh - bubbleGap)

	ly := layout{
		gopherX:     gopherX,
		gopherY:     gopherY,
		gopherScale: scale,
		bubbleX:     bx32,
		bubbleY:     by32,
		bubbleW:     float32(bw),
		bubbleH:     float32(bh),
		lines:       lines,
		lineHeight:  lineH,
	}
	return ly, sw, sh
}

// --- Game 生成 ---

var _ ebiten.Game = (*Game)(nil)

// Game はアプリケーションの状態を保持する。
type Game struct {
	gopherImage  *ebiten.Image
	fontFace     text.Face
	goFace       font.Face
	screenWidth  int
	screenHeight int
	layout       layout
	hasMessage   bool        // メッセージが存在するか
	msgTimer     int         // メッセージ表示残りフレーム数（0で消える）
	msgCh        chan string // 標準入力からのメッセージ受信チャネル

	// ドラッグ用状態
	dragging   bool
	dragStartX int
	dragStartY int
}

// NewGame は Game を初期化する。標準入力からのメッセージ受信を開始する。
func NewGame() (*Game, error) {
	img, err := loadGopherImage()
	if err != nil {
		return nil, err
	}
	goFace, err := loadFontFace()
	if err != nil {
		return nil, err
	}

	// 初期状態：メッセージなしのレイアウト
	ly, sw, sh := calcLayout(img, goFace, "")

	msgCh := make(chan string, 1)

	// 標準入力から行を読み取るgoroutine
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				msgCh <- line
			}
		}
	}()

	return &Game{
		gopherImage:  img,
		fontFace:     text.NewGoXFace(goFace),
		goFace:       goFace,
		screenWidth:  sw,
		screenHeight: sh,
		layout:       ly,
		msgCh:        msgCh,
	}, nil
}

// --- 描画 ---

func (gm *Game) Update() error {
	// 標準入力からの新しいメッセージをチェック
	select {
	case msg := <-gm.msgCh:
		message := strings.ReplaceAll(msg, "\\n", "\n")
		message = wrapText(message, gm.goFace, maxLineWidth)
		ly, sw, sh := calcLayout(gm.gopherImage, gm.goFace, message)

		// ウィンドウの右下位置を維持するよう位置を調整
		wx, wy := ebiten.WindowPosition()
		wx += gm.screenWidth - sw
		wy += gm.screenHeight - sh

		gm.layout = ly
		gm.screenWidth = sw
		gm.screenHeight = sh
		gm.hasMessage = true
		// 1文字につき2秒（60FPS基準）
		gm.msgTimer = len([]rune(message)) * ebiten.TPS()
		ebiten.SetWindowSize(sw, sh)
		ebiten.SetWindowPosition(wx, wy)
	default:
	}

	// メッセージ表示タイマーのカウントダウン
	if gm.hasMessage && gm.msgTimer > 0 {
		gm.msgTimer--
		if gm.msgTimer <= 0 {
			gm.hasMessage = false
			// メッセージなしのレイアウトに戻す
			ly, sw, sh := calcLayout(gm.gopherImage, gm.goFace, "")
			wx, wy := ebiten.WindowPosition()
			wx += gm.screenWidth - sw
			wy += gm.screenHeight - sh
			gm.layout = ly
			gm.screenWidth = sw
			gm.screenHeight = sh
			ebiten.SetWindowSize(sw, sh)
			ebiten.SetWindowPosition(wx, wy)
		}
	}

	ly := gm.layout
	cx, cy := ebiten.CursorPosition()

	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !gm.dragging {
			// Gopherの矩形内をクリックしたらドラッグ開始
			scale := ly.gopherScale
			w := float64(gm.gopherImage.Bounds().Dx()) * scale
			h := float64(gm.gopherImage.Bounds().Dy()) * scale
			if float64(cx) >= ly.gopherX && float64(cx) <= ly.gopherX+w &&
				float64(cy) >= ly.gopherY && float64(cy) <= ly.gopherY+h {
				gm.dragging = true
				gm.dragStartX = cx
				gm.dragStartY = cy
			}
		} else {
			// ドラッグ中：ウィンドウを移動
			dx := cx - gm.dragStartX
			dy := cy - gm.dragStartY
			if dx != 0 || dy != 0 {
				wx, wy := ebiten.WindowPosition()
				ebiten.SetWindowPosition(wx+dx, wy+dy)
			}
		}
	} else {
		gm.dragging = false
	}

	return nil
}

func (gm *Game) Draw(screen *ebiten.Image) {
	screen.Clear()

	ly := gm.layout

	if !gm.dragging && gm.hasMessage {
		gm.drawBubble(screen, ly)
		gm.drawText(screen, ly)
	}

	gm.drawGopher(screen, ly)
}

// drawBubble は角丸の吹き出し本体としっぽを描画する。
func (gm *Game) drawBubble(screen *ebiten.Image, ly layout) {
	bx, by, bw, bh := ly.bubbleX, ly.bubbleY, ly.bubbleW, ly.bubbleH
	r := float32(bubbleRadius)

	// 角丸四角形パス
	var bp vector.Path
	bp.MoveTo(bx+r, by)
	bp.LineTo(bx+bw-r, by)
	bp.ArcTo(bx+bw, by, bx+bw, by+r, r)
	bp.LineTo(bx+bw, by+bh-r)
	bp.ArcTo(bx+bw, by+bh, bx+bw-r, by+bh, r)
	bp.LineTo(bx+r, by+bh)
	bp.ArcTo(bx, by+bh, bx, by+bh-r, r)
	bp.LineTo(bx, by+r)
	bp.ArcTo(bx, by, bx+r, by, r)
	bp.Close()

	// しっぽ（吹き出し下部から小さく突き出る左向き曲線）
	tbx := bx + bw*0.65 // しっぽ基部のX中心
	tby := by + bh - 1  // しっぽ基部のY
	ttx := tbx - 15     // しっぽ先端X
	tty := tby + 20     // しっぽ先端Y

	tailCurve := func(p *vector.Path) {
		p.MoveTo(tbx-10, tby)
		p.QuadTo(tbx-8, tby+8, ttx, tty)
		p.QuadTo(tbx+2, tby+12, tbx+10, tby)
	}

	var tp vector.Path
	tailCurve(&tp)
	tp.Close()

	// 描画順序: 吹き出し塗り → しっぽ塗り → 吹き出し枠 → 境界消し → しっぽ外枠
	aa := &vector.DrawPathOptions{AntiAlias: true}

	vector.FillPath(screen, &bp, nil, aa)
	vector.FillPath(screen, &tp, nil, aa)

	vector.StrokePath(screen, &bp, &vector.StrokeOptions{Width: strokeWidth}, &vector.DrawPathOptions{
		AntiAlias: true, ColorScale: blackColorScale(),
	})

	// 境界の枠線を白で上書き
	vector.FillRect(screen, tbx-9, tby-2, 18, 4, color.White, true)

	// しっぽの外側の曲線のみ描画
	var to vector.Path
	tailCurve(&to)
	vector.StrokePath(screen, &to, &vector.StrokeOptions{
		Width: strokeWidth, LineCap: vector.LineCapRound, LineJoin: vector.LineJoinRound,
	}, &vector.DrawPathOptions{
		AntiAlias: true, ColorScale: blackColorScale(),
	})
}

// drawText は吹き出し内にメッセージを描画する。
func (gm *Game) drawText(screen *ebiten.Image, ly layout) {
	textH := float64(len(ly.lines)) * ly.lineHeight
	x := float64(ly.bubbleX) + bubblePadX/2 - 2
	// フォントのアセンダー分を補正して視覚的に上下均等にする
	y := float64(ly.bubbleY) + (float64(ly.bubbleH)-textH)/2 - 6

	for i, line := range ly.lines {
		op := &text.DrawOptions{}
		op.GeoM.Translate(x, y+float64(i)*ly.lineHeight)
		op.ColorScale.Scale(0, 0, 0, 1)
		text.Draw(screen, line, gm.fontFace, op)
	}
}

// drawGopher はGopher画像を描画する。
func (gm *Game) drawGopher(screen *ebiten.Image, ly layout) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(ly.gopherScale, ly.gopherScale)
	op.GeoM.Translate(ly.gopherX, ly.gopherY)
	screen.DrawImage(gm.gopherImage, op)
}

// blackColorScale は黒色の ColorScale を返す。
func blackColorScale() ebiten.ColorScale {
	var cs ebiten.ColorScale
	cs.Scale(0, 0, 0, 1)
	return cs
}

func (gm *Game) Layout(_, _ int) (int, int) {
	return gm.screenWidth, gm.screenHeight
}
