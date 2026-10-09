package score

// DiffName 是难度和文字的映射
func DiffName(d string) string {
	switch d {
	case "PST":
		return "Past"
	case "PRS":
		return "Present"
	case "FTR":
		return "Future"
	case "BYD":
		return "Beyond"
	case "ETR":
		return "Eternal"
	}
	return d
}
