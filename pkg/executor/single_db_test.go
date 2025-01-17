/*
 * Copyright 2022 CECTC, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package executor

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/cectc/dbpack/pkg/config"
	"github.com/cectc/dbpack/pkg/constant"
	"github.com/cectc/dbpack/pkg/mysql"
	"github.com/cectc/dbpack/pkg/proto"
	"github.com/cectc/dbpack/pkg/resource"
	"github.com/cectc/dbpack/pkg/visitor"
	"github.com/cectc/dbpack/testdata"
	"github.com/cectc/dbpack/third_party/parser"
)

func TestSingleDBExecutor(t *testing.T) {
	testCases := []*struct {
		query              bool
		connectionID       uint32
		sql                string
		inLocalTransaction bool
	}{
		{
			true,
			1,
			"select id, name, age from employee",
			false,
		},
		{
			true,
			1,
			"start transaction",
			true,
		},
		{
			false,
			1,
			"update employee set name = ? and age = ? and id = ?",
			true,
		},
		{
			true,
			1,
			"delete from employee where id = 1",
			true,
		},
		{
			true,
			1,
			"commit",
			false,
		},
		{
			true,
			1,
			"select id, name, age from employee",
			false,
		},
		{
			false,
			1,
			"update employee set name = ? and age = ? and id = ?",
			false,
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := testdata.NewMockDB(ctrl)
	tx := testdata.NewMockTx(ctrl)
	db.EXPECT().Query(gomock.Any(), gomock.Any()).Return(&mysql.Result{}, uint16(0), nil).MaxTimes(10)
	db.EXPECT().ExecuteStmt(gomock.Any(), gomock.Any()).Return(&mysql.Result{}, uint16(0), nil).MaxTimes(10)
	tx.EXPECT().Query(gomock.Any(), gomock.Any()).Return(&mysql.Result{}, uint16(0), nil)
	tx.EXPECT().ExecuteStmt(gomock.Any(), gomock.Any()).Return(&mysql.Result{}, uint16(0), nil)
	db.EXPECT().Begin(gomock.Any()).Return(tx, &mysql.Result{}, nil)
	tx.EXPECT().Commit(gomock.Any()).Return(&mysql.Result{}, nil)

	manager := testdata.NewMockDBManager(ctrl)
	manager.EXPECT().GetDB(gomock.Any()).AnyTimes().Return(db)
	resource.SetDBManager(manager)

	executor, err := NewSingleDBExecutor(&config.Executor{
		Name: "singledb",
		Mode: config.SDB,
		Config: map[string]interface{}{
			"data_source_ref": "employee",
		},
	})
	assert.Nil(t, err)

	for _, c := range testCases {
		t.Run(c.sql, func(t *testing.T) {
			p := parser.New()
			stmt, err := p.ParseOneStmt(c.sql, "", "")
			assert.Nil(t, err)
			stmt.Accept(&visitor.ParamVisitor{})

			if c.query {
				ctx := proto.WithVariableMap(context.Background())
				ctx = proto.WithConnectionID(ctx, c.connectionID)
				ctx = proto.WithCommandType(ctx, constant.ComQuery)
				ctx = proto.WithQueryStmt(ctx, stmt)

				result, warns, err := executor.ExecutorComQuery(ctx, c.sql)
				assert.Nil(t, err)
				assert.Equal(t, uint16(0), warns)
				assert.Equal(t, &mysql.Result{}, result)

				inLocalTransaction := executor.InLocalTransaction(ctx)
				assert.Equal(t, c.inLocalTransaction, inLocalTransaction)
			} else {
				protoStmt := &proto.Stmt{
					ParamsCount: 3,
					StmtNode:    stmt,
				}
				ctx := proto.WithVariableMap(context.Background())
				ctx = proto.WithConnectionID(ctx, c.connectionID)
				ctx = proto.WithCommandType(ctx, constant.ComStmtExecute)
				ctx = proto.WithPrepareStmt(ctx, protoStmt)

				result, warns, err := executor.ExecutorComStmtExecute(ctx, protoStmt)
				assert.Nil(t, err)
				assert.Equal(t, uint16(0), warns)
				assert.Equal(t, &mysql.Result{}, result)

				inLocalTransaction := executor.InLocalTransaction(ctx)
				assert.Equal(t, c.inLocalTransaction, inLocalTransaction)
			}
		})
	}
}
