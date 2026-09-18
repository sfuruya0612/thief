package aws

import (
	"strings"
	"sync"
	"time"
)

// EC2CountSampleInterval は Running な EC2 インスタンス数を定期的にサンプリングする間隔。
// 7 日の期間の粒度 (300 秒) と同じにし、1 日で 288 点、30 日で 8,640 点になる。
const EC2CountSampleInterval = 5 * time.Minute

// ec2CountRingSize は profile と region の組ごとに保持する記録の最大点数。
// 記録の契機は 2 つあり、一覧を AWS から取得したとき (handleEC2) と、一定間隔の
// 定期サンプリング (runEC2CountSampler) である。定期サンプリングだけで最長の期間
// (30 日) を覆える必要があるため、30 日をサンプリング間隔で割った点数 (8,640) の
// 2 倍を持たせ、Refresh で増える点と合わせても 30 日分が残るようにする。
const ec2CountRingSize = 2 * int(30*24*time.Hour/EC2CountSampleInterval)

// EC2CountRecorder は Running な EC2 インスタンス数の推移を、profile と region の組ごとに
// プロセス内のリングバッファへ記録する。
//
// AWS/EC2 名前空間にはアカウント全体の台数を表す標準メトリクスが無いため、一覧 API が
// AWS から取得した結果をその場で数え、加えて起動中は runEC2CountSampler が一定間隔で
// 一覧を取得して数えた値を残す。プロセスを再起動すると履歴は消える
// (thief は利用者の手元で起動する API サーバであり、常時起動を前提にしていない)。
type EC2CountRecorder struct {
	mu    sync.Mutex
	rings map[string]*ec2CountRing
}

// NewEC2CountRecorder は空の記録器を返す。
func NewEC2CountRecorder() *EC2CountRecorder {
	return &EC2CountRecorder{rings: map[string]*ec2CountRing{}}
}

// Record は profile と region の組に台数 1 点を追記する。呼び出し元は一覧を AWS から
// 取得した handleEC2 と、定期サンプリングの runEC2CountSampler である。一覧のキャッシュ
// から応答を返したとき (handleEC2 のキャッシュ HIT) に呼んではならない (同じ値が観測
// 時刻だけ変えて並び、推移を歪めるため)。
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

// EC2CountTarget はサンプリングと記録の対象になる profile と region の組。
type EC2CountTarget struct {
	Profile string
	Region  string
}

// Keys は記録を持つ profile と region の組を返す。順序は保証しない。
// 定期サンプリングが対象を決めるために使う (利用者が一度開いた組だけを回す)。
func (r *EC2CountRecorder) Keys() []EC2CountTarget {
	r.mu.Lock()
	defer r.mu.Unlock()
	targets := make([]EC2CountTarget, 0, len(r.rings))
	for key := range r.rings {
		profile, region, _ := strings.Cut(key, "\x00")
		targets = append(targets, EC2CountTarget{Profile: profile, Region: region})
	}
	return targets
}

// Series は profile と region の組の記録のうち、窓 w に収まる点を記録した順
// (観測時刻の昇順) で返す。
// 窓を呼び出し側から受け取るのは、応答が返す窓と絞り込みの境界を同じ値にするためである。
// 記録は一覧の取得と定期サンプリングで増えるため時刻は等間隔ではない。CloudWatch 由来の系列と違い
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
