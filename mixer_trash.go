package mixer


//func (mix *ServeMixer) find1(pattern string) []string {
//	var pos int
//	var j int
//
//	parts := make([]string, 3)
//
//	for i, p := range pattern[1:] + "/" {
//		if p == '/' {
//			if pos == i {
//				parts[j] = "/"
//			} else {
//				parts[j] = pattern[pos+1:i+1]
//			}
//			j++
//			pos = i+1
//		}
//	}
//
//	return parts
//}
//
//func (mix *ServeMixer) find2(pattern string) []string {
//	var pos int
//	var j int
//
//	parts := make([]string, 3)
//
//	for i, p := range pattern {
//		if p == '/' {
//			if pos != i {
//				parts[j] = pattern[pos:i]
//				j++
//			}
//			pos = i + 1
//			//continue
//		}
//		if len(pattern)-1 == i {
//			if pos-1 == i {
//				parts[j] = "/"
//			} else {
//				parts[j] = pattern[pos:]
//			}
//			j++
//		}
//	}
//
//	return parts
//}
//
//func (mix *ServeMixer) find3(pattern string) []string {
//}
