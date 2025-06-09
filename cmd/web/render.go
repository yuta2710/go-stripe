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
	var t *template.Template
	var err error

	templateToRender := fmt.Sprintf("templates/%s.page.tmpl", page)

	_, templateInMap := app.templateCache[templateToRender]

	if app.config.env == "production" && templateInMap {
		t = app.templateCache[templateToRender]
	} else {
		t, err = app.parseTemplate(partials, page, templateToRender)

		if err != nil {
			app.errorLog.Println(err)
			return err
		}
	}

	if td == nil {
		td = &templateData{}
	}

	td = app.addDefaultData(td, r)

	err = t.Execute(w, td)

	if err != nil {
		app.errorLog.Println(err)
		return err
	}

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
	var t *template.Template
	var err error

	// build partials
	if len(partials) > 0 {
		for i, x := range partials {
			partials[i] = fmt.Sprintf("templates/%s.partial.tmpl", x)
		}
	}

	if len(partials) > 0 {
		t, err = template.
			New(fmt.Sprintf("%s.page.tmpl", page)).
			Funcs(functions).
			ParseFS(templateFS, "base.layout.tmpl", strings.Join(partials, ","), templateToRender)
	} else {
		t, err = template.
			New(fmt.Sprintf("%s.page.tmpl", page)).
			Funcs(functions).
			ParseFS(templateFS, "base.layout.tmpl", templateToRender)
	}

	if err != nil {
		app.errorLog.Println(err)
		return nil, err
	}

	app.templateCache[templateToRender] = t

	return t, nil
}
