// InfoMark - a platform for managing courses with
//            distributing exercise sheets and testing exercise submissions
// Copyright (C) 2019  ComputerGraphics Tuebingen
// Authors: Patrick Wieschollek
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package app

import (
  "context"
  _ "fmt"
  "net/http"
  "strconv"

  "github.com/cgtuebingen/infomark-backend/model"
  "github.com/go-chi/chi"
  "github.com/go-chi/render"
  validation "github.com/go-ozzo/ozzo-validation"
)

// CourseStore specifies required database queries for course management.
type CourseStore interface {
  Get(courseID int64) (*model.Course, error)
  Update(p *model.Course) error
  GetAll() ([]model.Course, error)
  Create(p *model.Course) (*model.Course, error)
  Delete(courseID int64) error
}

// CourseResource specifies course management handler.
type CourseResource struct {
  CourseStore CourseStore
  SheetStore  SheetStore
}

// NewCourseResource create and returns a CourseResource.
func NewCourseResource(courseStore CourseStore, sheetStore SheetStore) *CourseResource {
  return &CourseResource{
    CourseStore: courseStore,
    SheetStore:  sheetStore,
  }
}

// .............................................................................

// courseRequest is the request payload for course management.
type courseRequest struct {
  *model.Course
  ProtectedID int64 `json:"id"`
}

// courseResponse is the response payload for course management.
type courseResponse struct {
  *model.Course
  Sheets []model.Sheet `json:"sheets"`
}

// newCourseResponse creates a response from a course model.
func (rs *CourseResource) newCourseResponse(p *model.Course) *courseResponse {

  sheets, _ := rs.SheetStore.SheetsOfCourse(p, false)

  return &courseResponse{
    Course: p,
    Sheets: sheets,
  }
}

// newCourseListResponse creates a response from a list of course models.
func (rs *CourseResource) newCourseListResponse(courses []model.Course) []render.Renderer {
  // https://stackoverflow.com/a/36463641/7443104
  list := []render.Renderer{}
  for k := range courses {
    list = append(list, rs.newCourseResponse(&courses[k]))
  }

  return list
}

// Bind preprocesses a courseRequest.
func (body *courseRequest) Bind(r *http.Request) error {
  // Sending the id via request-body is invalid.
  // The id should be submitted in the url.
  body.ProtectedID = 0

  err := validation.ValidateStruct(body,
    validation.Field(&body.Name, validation.Required),
  )
  return err

}

// Render post-processes a courseResponse.
func (body *courseResponse) Render(w http.ResponseWriter, r *http.Request) error {
  return nil
}

// IndexHandler is the enpoint for retrieving all courses if claim.root is true.
func (rs *CourseResource) IndexHandler(w http.ResponseWriter, r *http.Request) {
  // fetch collection of courses from database
  courses, err := rs.CourseStore.GetAll()

  // render JSON reponse
  if err = render.RenderList(w, r, rs.newCourseListResponse(courses)); err != nil {
    render.Render(w, r, ErrRender(err))
    return
  }
}

// CreateHandler is the enpoint for retrieving all courses if claim.root is true.
func (rs *CourseResource) CreateHandler(w http.ResponseWriter, r *http.Request) {
  // start from empty Request
  data := &courseRequest{}

  // parse JSON request into struct
  if err := render.Bind(r, data); err != nil {
    render.Render(w, r, ErrBadRequestWithDetails(err))
    return
  }

  // validate final model
  if err := data.Course.Validate(); err != nil {
    render.Render(w, r, ErrBadRequestWithDetails(err))
    return
  }

  // create course entry in database
  newCourse, err := rs.CourseStore.Create(data.Course)
  if err != nil {
    render.Render(w, r, ErrRender(err))
    return
  }

  // return course information of created entry
  if err := render.Render(w, r, rs.newCourseResponse(newCourse)); err != nil {
    render.Render(w, r, ErrRender(err))
    return
  }

  render.Status(r, http.StatusCreated)
}

// GetHandler is the enpoint for retrieving a specific course.
func (rs *CourseResource) GetHandler(w http.ResponseWriter, r *http.Request) {
  // `course` is retrieved via middle-ware
  course := r.Context().Value("course").(*model.Course)

  // render JSON reponse
  if err := render.Render(w, r, rs.newCourseResponse(course)); err != nil {
    render.Render(w, r, ErrRender(err))
    return
  }

  render.Status(r, http.StatusOK)
}

// PatchHandler is the endpoint fro updating a specific course with given id.
func (rs *CourseResource) EditHandler(w http.ResponseWriter, r *http.Request) {
  // start from empty Request
  data := &courseRequest{
    Course: r.Context().Value("course").(*model.Course),
  }

  // parse JSON request into struct
  if err := render.Bind(r, data); err != nil {
    render.Render(w, r, ErrBadRequestWithDetails(err))
    return
  }

  // update database entry
  if err := rs.CourseStore.Update(data.Course); err != nil {
    render.Render(w, r, ErrInternalServerErrorWithDetails(err))
    return
  }

  render.Status(r, http.StatusNoContent)
}

func (rs *CourseResource) DeleteHandler(w http.ResponseWriter, r *http.Request) {
  course := r.Context().Value("course").(*model.Course)

  // update database entry
  if err := rs.CourseStore.Delete(course.ID); err != nil {
    render.Render(w, r, ErrInternalServerErrorWithDetails(err))
    return
  }

  render.Status(r, http.StatusOK)
}

// .............................................................................
// Context middleware is used to load an Course object from
// the URL parameter `courseID` passed through as the request. In case
// the Course could not be found, we stop here and return a 404.
// We do NOT check whether the course is authorized to get this course.
func (d *CourseResource) Context(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // TODO: check permission if inquirer of request is allowed to access this course
    // Should be done via another middleware
    var course_id int64
    var err error

    // try to get id from URL
    if course_id, err = strconv.ParseInt(chi.URLParam(r, "courseID"), 10, 64); err != nil {
      render.Render(w, r, ErrNotFound)
      return
    }

    // find specific course in database
    course, err := d.CourseStore.Get(course_id)
    if err != nil {
      render.Render(w, r, ErrNotFound)
      return
    }

    // serve next
    ctx := context.WithValue(r.Context(), "course", course)
    next.ServeHTTP(w, r.WithContext(ctx))
  })
}
