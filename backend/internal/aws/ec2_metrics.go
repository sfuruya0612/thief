package aws

import (
	"sync"
	"time"
)

// ec2CountRingSize は profile と region の組ごとに保持する記録の最大点数。
// 記録は一覧を AWS から実際に取得したときだけ増えるため、キャッシュ TTL (1 時間) で
// 30 日を埋めても 720 点にしかならない。Refresh の連打で点が増えても最長の期間
// (30 日) を覆えるだけの余裕を持たせた値にする。
const ec2CountRingSize = 4096

// EC2CountRecorder は Running な EC2 インスタンス数の推移を、profile と region の組ごとに
// プロセス内のリングバッファへ記録する。
//
// AWS/EC2 名前空間にはアカウント全体の台数を表す標準メトリクスが無いため、一覧 API が
// AWS から取得した結果をその場で数えて残す。プロセスを再起動すると履歴は消える
// (thief は利用者の手元で起動する API サーバであり、常時起動を前提にしていない)。
type EC2CountRecorder struct {
	mu    sync.Mutex
	rings map[string]*ec2CountRing
}

// NewEC2CountRecorder は空の記録器を返す。
func NewEC2CountRecorder() *EC2CountRecorder {
	return &EC2CountRecorder{rings: map[string]*ec2CountRing{}}
}

// Record は profile と region の組に台数 1 点を追記する。キャッシュから応答を返した
// ときに呼んではならない (同じ値が観測時刻だけ変えて並び、推移を歪めるため)。
func (r *EC2CountRecorder) Record(profile, region string, count int, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ec2CountKey(profile, region)
	ring, ok := r.rings[key]
	if !ok {
		ring = &ec2CountRing{}
		r.rings[key] = ring
	}
	value := float64(count)
	ring.add(MetricPoint{T: at.UnixMilli(), V: &value})
}

// Series は profile と region の組の記録のうち、窓 w に収まる点を記録した順
// (観測時刻の昇順) で返す。
// 窓を呼び出し側から受け取るのは、応答が返す窓と絞り込みの境界を同じ値にするためである。
// 記録は一覧の取得ごとに増えるため時刻は等間隔ではない。CloudWatch 由来の系列と違い
// グリッドへ並べ直さないのは、観測していない時刻を欠測として捏造しないためである。
func (r *EC2CountRecorder) Series(profile, region string, w TimeseriesWindow) []MetricPoint {
	r.mu.Lock()
	defer r.mu.Unlock()
	ring, ok := r.rings[ec2CountKey(profile, region)]
	if !ok {
		return []MetricPoint{}
	}
	points := make([]MetricPoint, 0, ec2CountRingSize)
	for _, p := range ring.snapshot() {
		// 両端を含めて絞る。終端も見るのは、窓を確定した後に別リクエストの一覧取得が
		// Record を呼ぶと終端より後の点が生じ、応答の窓 (X 軸の範囲) の外に点が混ざるためである。
		// 時計の後退で直近の点が終端より後になった場合も落とすが、X 軸の max は窓の終端なので
		// その点は応答に入れても描かれず、表示は変わらない。時計が追い付いた次の応答から戻る。
		// 終端を最新の点までずらすと窓の幅が期間と一致しなくなるため、そうしない。
		if p.T >= w.Start && p.T <= w.End {
			points = append(points, p)
		}
	}
	return points
}

// ec2CountKey は profile と region の組をリングバッファのキーにする。
// 区切りに使う "\x00" は profile 名にも region 名にも現れないため、異なる組が同じキーに
// 衝突しない。
func ec2CountKey(profile, region string) string {
	return profile + "\x00" + region
}

// ec2CountRing は固定長のリングバッファ。上限に達したあとは最古の点を上書きする。
type ec2CountRing struct {
	points [ec2CountRingSize]MetricPoint
	// next は次に書き込む位置。full が true のときは最古の点の位置でもある。
	next int
	full bool
}

func (b *ec2CountRing) add(p MetricPoint) {
	b.points[b.next] = p
	b.next = (b.next + 1) % ec2CountRingSize
	if b.next == 0 {
		b.full = true
	}
}

// snapshot は保持している点を古い順に複製して返す。
func (b *ec2CountRing) snapshot() []MetricPoint {
	if !b.full {
		return append([]MetricPoint(nil), b.points[:b.next]...)
	}
	out := make([]MetricPoint, 0, ec2CountRingSize)
	out = append(out, b.points[b.next:]...)
	return append(out, b.points[:b.next]...)
}

// CountRunningEC2 は一覧のうち Running 状態のインスタンス数を返す。
// 状態の表記ゆれは ResourceState (NormalizeState) が吸収する。
func CountRunningEC2(resources []EC2Resource) int {
	n := 0
	for _, r := range resources {
		if r.ResourceState() == "running" {
			n++
		}
	}
	return n
}
