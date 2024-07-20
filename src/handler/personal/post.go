// Copyright 2019 HenryYee.
//
// Licensed under the AGPL, Version 3.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.gnu.org/licenses/agpl-3.0.en.html
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package personal

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/handler/manage/tpl"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib"
	"Yearning-go/src/model"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"github.com/cookieY/yee"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strings"
	"time"
)

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func Post(c yee.Context) (err error) {
	switch c.Params("tp") {
	case "post":
		return sqlOrderPost(c)
	case "edit":
		return PersonalUserEdit(c)
	}
	return err
}

func sqlOrderPost(c yee.Context) (err error) {

	body, err := io.ReadAll(c.Request().Body)
	var requestBody map[string]interface{}
	err = json.Unmarshal(body, &requestBody)
	sourceIDList := requestBody["source_id_list"].([]interface{})
	sourceList := requestBody["source_list"].([]interface{})
	var sourceIDListStr []string
	for _, sourceId := range sourceIDList {
		if idStr, ok := sourceId.(string); ok {
			sourceIDListStr = append(sourceIDListStr, idStr)
		}
	}

	var sourceListStr []string
	for _, source := range sourceList {
		if idStr, ok := source.(string); ok {
			sourceListStr = append(sourceListStr, idStr)
		}
	}

	// 重新放到body里面
	c.Request().Body = io.NopCloser(bytes.NewBuffer(body))

	u := new(model.CoreSqlOrder)
	user := new(lib.Token).JwtParse(c)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_REQ_BIND)
	}
	length, err := wrapperPostOrderInfo(u, c)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	u.ID = 0
	model.DB().Create(u)
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   u.WorkId,
		Username: user.Username,
		Action:   i18n.DefaultLang.Load(i18n.INFO_SUBMITTED),
		Time:     time.Now().Format("2006-01-02 15:04"),
	})

	/* create batch */
	isHasSource := contains(sourceIDListStr, u.SourceId)
	if !isHasSource {
		sourceIDListStr = append(sourceIDListStr, u.SourceId)
		sourceListStr = append(sourceListStr, u.Source)
	}

	var workId = u.WorkId
	for i, sourceId := range sourceIDListStr {

		/* 生成子工作流对象 */
		subU := *u
		/**重新生成work id**/
		tmpUUID := uuid.New()
		hash := md5.New()
		hash.Write([]byte(tmpUUID.String()))
		hashBytes := hash.Sum(nil)
		hashString := hex.EncodeToString(hashBytes)
		hashStringPrefix6 := hashString[:6]

		subU.ID = 0
		subU.WorkId = "0-" + workId + "-" + hashStringPrefix6
		subU.Source = sourceListStr[i]
		subU.SourceId = sourceId
		model.DB().Create(&subU)
	}
	/* end */

	lib.MessagePush(u.WorkId, 2, "")

	if u.Type == lib.DML {
		CallAutoTask(u, length)
	}

	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.ORDER_POST_SUCCESS)))
}

func wrapperPostOrderInfo(order *model.CoreSqlOrder, y yee.Context) (length int, err error) {
	var from model.CoreWorkflowTpl
	var flowId model.CoreDataSource
	var step []tpl.Tpl
	model.DB().Model(model.CoreDataSource{}).Where("source_id = ?", order.SourceId).First(&flowId)
	model.DB().Model(model.CoreWorkflowTpl{}).Where("id =?", flowId.FlowID).Find(&from)
	err = json.Unmarshal(from.Steps, &step)
	if err != nil || len(step) < 2 {
		y.Logger().Error(err)
		return 0, err
	}
	user := new(lib.Token).JwtParse(y)
	w := lib.GenWorkid()
	if order.Source == "" {
		order.Source = flowId.Source
	}
	if order.IDC == "" {
		order.IDC = flowId.IDC
	}
	order.WorkId = w
	order.Username = user.Username
	order.RealName = user.RealName
	order.Date = time.Now().Format("2006-01-02 15:04")
	order.Status = 2
	order.Time = time.Now().Format("2006-01-02")
	order.CurrentStep = 1
	order.Assigned = strings.Join(step[1].Auditor, ",")
	order.Relevant = lib.JsonStringify(order.Relevant)
	return len(step), nil
}
