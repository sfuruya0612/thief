package accesslog

import "errors"

var (
	// ErrInvalidTimeRange は期間の指定が不正であることを表す。
	ErrInvalidTimeRange = errors.New("invalid time range")
	// ErrRangeTooLong は期間が上限を超えていることを表す。
	ErrRangeTooLong = errors.New("time range too long")
	// ErrInvalidSource はソースの指定が不正であることを表す。
	ErrInvalidSource = errors.New("invalid access log source")
	// ErrSourceNotReady はソースのログが無効、または出力先が未対応であることを表す。
	ErrSourceNotReady = errors.New("access log source is not ready")
	// ErrAttributionRequired は共有された出力先でリソースを特定できないことを表す。
	ErrAttributionRequired = errors.New("access log source cannot attribute records to a resource")
	// ErrInvalidTable はテーブルの指定が不正であることを表す。
	ErrInvalidTable = errors.New("invalid athena table")
	// ErrUnsupportedProfile はスキーマプロファイルを判定できないことを表す。
	ErrUnsupportedProfile = errors.New("unsupported athena schema profile")
	// ErrInvalidFilter はフィルタの指定が不正であることを表す。
	ErrInvalidFilter = errors.New("invalid access log filter")
	// ErrOutputLocationRequired は Athena のクエリ結果の出力先が無いことを表す。
	ErrOutputLocationRequired = errors.New("athena query results output location is required")
)
