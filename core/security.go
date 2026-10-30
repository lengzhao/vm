package core

// SecurityReviewer 安全审查模块接口
type SecurityReviewer interface{ Review(string) error }

// SecurityError 安全审查错误
type SecurityError struct {
	Message    string
	ErrorType  SecurityErrorType
	ImportPath string
	Keyword    string
}

// SecurityErrorType 安全错误类型
type SecurityErrorType int

const (
	ImportNotAllowed SecurityErrorType = iota
	KeywordNotAllowed
	GlobalVarNotAllowed
)

func (e *SecurityError) Error() string {
	return e.Message
}
