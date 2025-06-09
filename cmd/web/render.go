package main

import (
	"embed"
	"fmt"
	"net/http"
	"strings"
	"text/template"
)

/*
templateData chứa dữ liệu được truyền vào các template HTML.
Nó bao gồm các dữ liệu chung như CSRF token, flash message, trạng thái xác thực người dùng,
cũng như dữ liệu cụ thể cho từng trang thông qua các map khác nhau.
*/
type templateData struct {
	StringMap       map[string]string
	IntMap          map[string]int
	FloatMap        map[string]float32
	Data            map[string]interface{}
	CSRFToken       string
	Flash           string
	Warning         string
	Error           string
	IsAuthenticated int
	API             string
	CSSVersion      string
}

/*
functions là một map chứa các hàm tùy chỉnh có thể được sử dụng trong template.
Hiện tại nó đang trống nhưng có thể được mở rộng với các hàm trợ giúp.
*/
var functions = template.FuncMap{}

/*
templateFS chứa hệ thống tệp được nhúng cho các template HTML.
Chỉ thị `//go:embed templates` nhúng nội dung của thư mục "templates"
vào trong file thực thi đã biên dịch, cho phép truy cập template
mà không cần dựa vào hệ thống tệp bên ngoài.
*/
// go:embed templates
var templateFS embed.FS

/*
addDefaultData thêm các dữ liệu mặc định vào templateData.
Các dữ liệu này thường là chung cho tất cả các template, ví dụ như URL API,
phiên bản CSS, hoặc thông tin dựa trên session.
*/
func (app *application) addDefaultData(td *templateData, r *http.Request) *templateData {
	return td
}

/*
renderTemplate render một trang HTML cụ thể cùng với các partials được chỉ định (nếu có).
Nó xử lý việc cache template trong môi trường production để cải thiện hiệu suất.

Tham số:

	w: http.ResponseWriter để ghi HTML đã render ra.
	r: *http.Request cho request hiện tại, được sử dụng cho context (ví dụ: bởi addDefaultData).
	page: Tên cơ sở của template trang cần render (ví dụ: "home" cho "home.page.tmpl").
	td: Con trỏ tới templateData chứa dữ liệu sẽ được truyền vào template. Nếu nil, một templateData mới sẽ được khởi tạo.
	partials: Một slice variadic các chuỗi, mỗi chuỗi là tên cơ sở của một partial template
			sẽ được bao gồm (ví dụ: "nav", "footer").

Trả về:

	Một error nếu việc parse hoặc thực thi template thất bại, ngược lại trả về nil.
*/
func (app *application) renderTemplate(w http.ResponseWriter, r *http.Request, page string, td *templateData, partials ...string) error {
	var t *template.Template // Khai báo biến t để lưu trữ template đã được parse hoặc lấy từ cache.
	var err error            // Khai báo biến err để bắt lỗi.

	// Tạo tên đầy đủ của file template trang cần render.
	// Ví dụ: nếu page là "home", templateToRender sẽ là "templates/home.page.tmpl".
	templateToRender := fmt.Sprintf("templates/%s.page.tmpl", page)

	// Kiểm tra xem template đã có trong cache hay chưa.
	// templateInMap sẽ là true nếu templateToRender tồn tại trong app.templateCache.
	_, templateInMap := app.templateCache[templateToRender]

	// Logic xử lý cache template:
	// Nếu ứng dụng đang chạy ở môi trường "production" VÀ template đã có trong cache,
	// thì sử dụng template từ cache.
	if app.config.env == "production" && templateInMap {
		t = app.templateCache[templateToRender]
	} else {
		// Ngược lại (môi trường không phải "production" HOẶC template chưa có trong cache),
		// tiến hành parse lại template.
		// Điều này hữu ích trong môi trường "development" để thấy thay đổi ngay lập tức
		// mà không cần khởi động lại ứng dụng.
		t, err = app.parseTemplate(partials, page, templateToRender)

		// Nếu có lỗi trong quá trình parse template, ghi log và trả về lỗi.
		if err != nil {
			app.errorLog.Println(err) // Ghi lại chi tiết lỗi vào errorLog.
			return err                // Trả về lỗi để hàm gọi có thể xử lý.
		}
		// Lưu ý: hàm parseTemplate sẽ tự động cập nhật cache nếu parse thành công.
	}

	// Nếu con trỏ td (templateData) là nil (nghĩa là không có dữ liệu cụ thể nào được truyền vào từ handler),
	// khởi tạo một đối tượng templateData mới.
	// Điều này đảm bảo td luôn có giá trị hợp lệ để truyền vào template và hàm addDefaultData.
	if td == nil {
		td = &templateData{}
	}

	// Thêm các dữ liệu mặc định (ví dụ: CSRF token, thông tin người dùng, URL API, phiên bản CSS) vào td.
	// Hàm này có thể tùy chỉnh td trước khi nó được sử dụng để render template.
	td = app.addDefaultData(td, r)

	// Thực thi template (render HTML) với dữ liệu td và ghi kết quả vào http.ResponseWriter (w).
	// Template t đã được parse (hoặc lấy từ cache) sẽ được "điền" dữ liệu từ td.
	err = t.Execute(w, td)

	// Nếu có lỗi trong quá trình thực thi template (ví dụ: lỗi cú pháp trong template,
	// hoặc lỗi khi ghi vào ResponseWriter), ghi log và trả về lỗi.
	if err != nil {
		app.errorLog.Println(err) // Ghi lại chi tiết lỗi.
		return err                // Trả về lỗi.
	}

	// Nếu không có lỗi nào xảy ra trong suốt quá trình, trả về nil để báo hiệu thành công.
	return nil

}

/*
parseTemplate parse một tập hợp các tệp template (layout cơ sở, trang, và các partials tùy chọn)
từ hệ thống tệp được nhúng (templateFS).
Nó xây dựng một template mới, liên kết nó với các hàm tùy chỉnh, và parse các tệp được chỉ định.
Template đã được parse sau đó được lưu trữ trong cache template của ứng dụng.

Tham số:

	partials: Một slice các chuỗi, mỗi chuỗi là tên cơ sở của một partial template
			(ví dụ: "nav" cho "templates/nav.partial.tmpl").
	page: Tên cơ sở của template trang chính (ví dụ: "home" cho "templates/home.page.tmpl").
	templateToRender: Đường dẫn đầy đủ của tệp template trang, được sử dụng làm khóa cho cache template
			(ví dụ: "templates/home.page.tmpl").

Trả về:

	Một con trỏ tới *template.Template đã được parse và một error nếu việc parse thất bại, ngược lại trả về nil.
*/
func (app *application) parseTemplate(partials []string, page, templateToRender string) (*template.Template, error) {
	var t *template.Template // Khai báo biến t để lưu trữ template đã được parse.
	var err error            // Khai báo biến err để bắt lỗi trong quá trình parse.

	// build partials: Chuẩn bị đường dẫn đầy đủ cho các file partial template.
	// Lưu ý: Đường dẫn này được xây dựng dựa trên giả định các file partial nằm trong thư mục "templates".
	if len(partials) > 0 { // Kiểm tra xem có partial template nào được truyền vào không.
		for i, x := range partials { // Duyệt qua danh sách tên các partial.
			// Cập nhật phần tử trong slice 'partials' thành đường dẫn đầy đủ.
			// Ví dụ: nếu x là "nav", partials[i] sẽ trở thành "templates/nav.partial.tmpl".
			// Quan trọng: Với `embed.FS` và `ParseFS`, đường dẫn nên là tương đối so với thư mục gốc đã embed (ví dụ: "nav.partial.tmpl" nếu "templates" là thư mục gốc).
			// Tuy nhiên, cách sử dụng `strings.Join` bên dưới có thể không hoạt động như mong đợi với `ParseFS` cho nhiều partials.
			partials[i] = fmt.Sprintf("templates/%s.partial.tmpl", x)
		}
	}

	if len(partials) > 0 { // Nếu có partial templates được chỉ định.
		// Bắt đầu một template mới, đặt tên theo file page (ví dụ: "home.page.tmpl").
		// Tên này quan trọng vì nó là tên mà các template khác (như base.layout.tmpl) sẽ dùng để {{define "page_name.page.tmpl"}}
		// Thêm các hàm tùy chỉnh (functions) vào template để có thể sử dụng trong các file .tmpl.
		// Parse các file từ templateFS (hệ thống file nhúng):
		// 1. "base.layout.tmpl": File layout cơ sở, thường chứa cấu trúc HTML chung.
		// 2. strings.Join(partials, ","): Nối tất cả các đường dẫn partial đã chuẩn bị ở trên thành một chuỗi duy nhất,
		//    phân tách bằng dấu phẩy.
		//    CẢNH BÁO: `ParseFS` mong đợi các tên file là các đối số chuỗi riêng biệt (variadic ...string).
		//    Việc truyền một chuỗi duy nhất như thế này có thể sẽ chỉ parse file đầu tiên hoặc gây lỗi,
		//    chứ không parse tất cả các partials như ý định.
		// 3. templateToRender: File page chính cần render (ví dụ: "templates/home.page.tmpl").
		t, err = template.
			New(fmt.Sprintf("%s.page.tmpl", page)).                                                // Đặt tên cho template chính, thường là tên file của page.
			Funcs(functions).                                                                      // Gắn các hàm helper vào template.
			ParseFS(templateFS, "base.layout.tmpl", strings.Join(partials, ","), templateToRender) // Parse các file template.
	} else { // Nếu không có partial templates nào được chỉ định.
		// Tương tự như trên, nhưng chỉ parse file layout cơ sở và file page chính.
		// Không có partials nào được bao gồm.
		t, err = template.
			New(fmt.Sprintf("%s.page.tmpl", page)).                   // Đặt tên cho template chính.
			Funcs(functions).                                         // Gắn các hàm helper.
			ParseFS(templateFS, "base.layout.tmpl", templateToRender) // Parse layout và page.
	}

	// Kiểm tra lỗi sau khi thực hiện ParseFS.
	if err != nil {
		app.errorLog.Println(err) // Ghi log lỗi nếu có lỗi xảy ra trong quá trình parse.
		return nil, err           // Trả về nil cho template và trả về lỗi đã xảy ra.
	}

	// Nếu parse thành công, lưu template đã parse vào cache của ứng dụng.
	// Khóa cache là 'templateToRender' (ví dụ: "templates/home.page.tmpl"),
	// giá trị là con trỏ tới template đã parse (t).
	app.templateCache[templateToRender] = t

	return t, nil // Trả về template đã parse thành công và không có lỗi (err là nil).
}
