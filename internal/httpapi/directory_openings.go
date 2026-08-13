package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"lumi/internal/directoryopener"

	"github.com/labstack/echo/v4"
)

type DirectoryOpener interface {
	Open(context.Context, string) error
}

type DirectoryOpeningHandler struct {
	opener DirectoryOpener
}

func NewDirectoryOpeningHandler(opener DirectoryOpener) *DirectoryOpeningHandler {
	return &DirectoryOpeningHandler{opener: opener}
}

type createDirectoryOpeningRequest struct {
	RootPath string `json:"root_path"`
}

func (handler *DirectoryOpeningHandler) Create(c echo.Context) error {
	var request createDirectoryOpeningRequest
	if err := decodeJSON(c, &request); err != nil {
		return err
	}
	if strings.TrimSpace(request.RootPath) == "" {
		return NewError(http.StatusUnprocessableEntity, "validation_failed", "打开位置参数无效", "root_path 不能为空。", nil)
	}
	if err := handler.opener.Open(c.Request().Context(), request.RootPath); err != nil {
		switch {
		case errors.Is(err, directoryopener.ErrNotFound):
			return NewError(http.StatusNotFound, "directory_not_found", "项目目录不存在", "目录可能已移动，或所在磁盘暂时离线。", err)
		case errors.Is(err, directoryopener.ErrNotDirectory):
			return NewError(http.StatusUnprocessableEntity, "directory_not_a_folder", "项目位置不是文件夹", "请重新定位项目目录后再试。", err)
		case errors.Is(err, directoryopener.ErrUnavailable):
			return NewError(http.StatusNotImplemented, "directory_opener_unavailable", "当前系统无法打开项目位置", "请在系统文件管理器中手动打开项目路径。", err)
		default:
			return NewError(http.StatusInternalServerError, "directory_open_failed", "无法打开项目位置", "请重试，或在系统文件管理器中手动打开项目路径。", err)
		}
	}
	return Success(c, http.StatusCreated, nil)
}
