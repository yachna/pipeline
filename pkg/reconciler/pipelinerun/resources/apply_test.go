/*
Copyright 2019 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/test/diff"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/selection"
)

func TestApplyParameters(t *testing.T) {
	for _, tt := range []struct {
		name     string
		original v1beta1.PipelineSpec
		params   []v1beta1.Param
		expected v1beta1.PipelineSpec
		alpha    bool
	}{{
		name: "single parameter",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param)")},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("second-value")},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
	}, {
		name: "parameter propagation string no task or task default winner pipeline",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Steps: []v1beta1.Step{{
							Name:   "step1",
							Image:  "ubuntu",
							Script: `#!/usr/bin/env bash\necho "$(params.HELLO)"`,
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "HELLO", Value: *v1beta1.NewArrayOrString("hello param!")}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Steps: []v1beta1.Step{{
							Name:   "step1",
							Image:  "ubuntu",
							Script: `#!/usr/bin/env bash\necho "hello param!"`,
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation array no task or task default winner pipeline",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "$(params.HELLO[*])"},
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "HELLO", Value: *v1beta1.NewArrayOrString("hello", "param", "!!!")}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "hello", "param", "!!!"},
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation object no task or task default winner pipeline",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "$(params.myObject.key1) $(params.myObject.key2)"},
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "myObject", Value: *v1beta1.NewObject(map[string]string{"key1": "hello", "key2": "world!"})}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "hello world!"},
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation with task default but no task winner pipeline",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:   "step1",
							Image:  "ubuntu",
							Script: `#!/usr/bin/env bash\necho "$(params.HELLO)"`,
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "HELLO", Value: *v1beta1.NewArrayOrString("pipeline param!")}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:   "step1",
							Image:  "ubuntu",
							Script: `#!/usr/bin/env bash\necho "pipeline param!"`,
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation array with task default but no task winner pipeline",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default", "param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "$(params.HELLO)"},
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "HELLO", Value: *v1beta1.NewArrayOrString("pipeline", "param!")}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default", "param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "pipeline", "param!"},
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation array with task default and task winner task",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "HELLO", Value: *v1beta1.NewArrayOrString("task", "param!")},
				},
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default", "param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "$(params.HELLO)"},
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "HELLO", Value: *v1beta1.NewArrayOrString("pipeline", "param!")}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "HELLO", Value: *v1beta1.NewArrayOrString("task", "param!")},
				},
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default", "param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "task", "param!"},
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation with task default and task winner task",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "HELLO", Value: *v1beta1.NewArrayOrString("task param!")},
				},
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:   "step1",
							Image:  "ubuntu",
							Script: `#!/usr/bin/env bash\necho "$(params.HELLO)"`,
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "HELLO", Value: *v1beta1.NewArrayOrString("pipeline param!")}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "HELLO", Value: *v1beta1.NewArrayOrString("task param!")},
				},
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name:    "HELLO",
							Default: v1beta1.NewArrayOrString("default param!"),
						}},
						Steps: []v1beta1.Step{{
							Name:   "step1",
							Image:  "ubuntu",
							Script: `#!/usr/bin/env bash\necho "task param!"`,
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation object with task default but no task winner pipeline",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name: "myobject",
							Properties: map[string]v1beta1.PropertySpec{
								"key1": {Type: "string"},
								"key2": {Type: "string"},
							},
							Default: v1beta1.NewObject(map[string]string{
								"key1": "default",
								"key2": "param!",
							}),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "$(params.myobject.key1) $(params.myobject.key2)"},
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "myobject", Value: *v1beta1.NewObject(map[string]string{
			"key1": "pipeline",
			"key2": "param!!",
		})}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name: "myobject",
							Properties: map[string]v1beta1.PropertySpec{
								"key1": {Type: "string"},
								"key2": {Type: "string"},
							},
							Default: v1beta1.NewObject(map[string]string{
								"key1": "default",
								"key2": "param!",
							}),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "pipeline param!!"},
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "parameter propagation object with task default and task winner task",
		original: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "myobject", Value: *v1beta1.NewObject(map[string]string{
						"key1": "task",
						"key2": "param!",
					})},
				},
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name: "myobject",
							Properties: map[string]v1beta1.PropertySpec{
								"key1": {Type: "string"},
								"key2": {Type: "string"},
							},
							Default: v1beta1.NewObject(map[string]string{
								"key1": "default",
								"key2": "param!!",
							}),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "$(params.myobject.key1) $(params.myobject.key2)"},
						}},
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "myobject", Value: *v1beta1.NewObject(map[string]string{"key1": "pipeline", "key2": "param!!!"})}},
		expected: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "myobject", Value: *v1beta1.NewObject(map[string]string{
						"key1": "task",
						"key2": "param!",
					})},
				},
				TaskSpec: &v1beta1.EmbeddedTask{
					TaskSpec: v1beta1.TaskSpec{
						Params: []v1beta1.ParamSpec{{
							Name: "myobject",
							Properties: map[string]v1beta1.PropertySpec{
								"key1": {Type: "string"},
								"key2": {Type: "string"},
							},
							Default: v1beta1.NewObject(map[string]string{
								"key1": "default",
								"key2": "param!!",
							}),
						}},
						Steps: []v1beta1.Step{{
							Name:  "step1",
							Image: "ubuntu",
							Args:  []string{"#!/usr/bin/env bash\n", "echo", "task param!"},
						}},
					},
				},
			}},
		},
		alpha: true,
	}, {
		name: "single parameter with when expression",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "$(params.first-param)",
					Operator: selection.In,
					Values:   []string{"$(params.second-param)"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "default-value",
					Operator: selection.In,
					Values:   []string{"second-value"},
				}},
			}},
		},
	}, {
		name: "object parameter with when expression",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
						"key3": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
						"key3": "val3",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "$(params.myobject.key1)",
					Operator: selection.In,
					Values:   []string{"$(params.myobject.key2)", "$(params.myobject.key3)"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "myobject", Value: *v1beta1.NewObject(map[string]string{
			"key1": "val1",
			"key2": "val2",
			"key3": "val1",
		})}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
						"key3": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
						"key3": "val3",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "val1",
					Operator: selection.In,
					Values:   []string{"val2", "val1"},
				}},
			}},
		},
	}, {
		name: "string pipeline parameter nested inside task parameter",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.$(params.first-param))")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.$(params.second-param))")},
				},
			}},
		},
		params: nil, // no parameter values.
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.default-value)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.default-value)")},
				},
			}},
		},
	}, {
		name: "array pipeline parameter nested inside task parameter",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default", "array", "value")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("firstelement", "$(params.first-param)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("firstelement", "$(params.second-param)")},
				},
			}},
		},
		params: []v1beta1.Param{
			{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value", "array")},
		},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default", "array", "value")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("firstelement", "default", "array", "value")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("firstelement", "second-value", "array")},
				},
			}},
		},
	}, {
		name: "object pipeline parameter nested inside task parameter",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.$(params.myobject.key1))")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.$(params.myobject.key2))")},
				},
			}},
		},
		params: nil, // no parameter values.
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.val1)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.val2)")},
				},
			}},
		},
	}, {
		name: "parameter evaluation with final tasks",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param)")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param)")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "$(params.first-param)",
					Operator: selection.In,
					Values:   []string{"$(params.second-param)"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("second-value")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "default-value",
					Operator: selection.In,
					Values:   []string{"second-value"},
				}},
			}},
		},
	}, {
		name: "parameter evaluation with both tasks and final tasks",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param)")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param)")},
				},
			}},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param)")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param)")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "$(params.first-param)",
					Operator: selection.In,
					Values:   []string{"$(params.second-param)"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("second-value")},
				},
			}},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("second-value")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "default-value",
					Operator: selection.In,
					Values:   []string{"second-value"},
				}},
			}},
		},
	}, {
		name: "object parameter evaluation with both tasks and final tasks",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.myobject.key1)")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.myobject.key2)")},
				},
			}},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.myobject.key1)")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.myobject.key2)")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "$(params.myobject.key1)",
					Operator: selection.In,
					Values:   []string{"$(params.myobject.key2)"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "myobject", Value: *v1beta1.NewObject(map[string]string{
			"key1": "foo",
			"key2": "bar",
		})}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("foo")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("bar")},
				},
			}},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("foo")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("bar")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
		},
	}, {
		name: "parameter references with bracket notation and special characters",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first.param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second/param", Type: v1beta1.ParamTypeString},
				{Name: "third.param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "fourth/param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString(`$(params["first.param"])`)},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString(`$(params["second/param"])`)},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString(`$(params['third.param'])`)},
					{Name: "first-task-fourth-param", Value: *v1beta1.NewArrayOrString(`$(params['fourth/param'])`)},
					{Name: "first-task-fifth-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
		params: []v1beta1.Param{
			{Name: "second/param", Value: *v1beta1.NewArrayOrString("second-value")},
			{Name: "fourth/param", Value: *v1beta1.NewArrayOrString("fourth-value")},
		},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first.param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second/param", Type: v1beta1.ParamTypeString},
				{Name: "third.param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "fourth/param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("second-value")},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "first-task-fourth-param", Value: *v1beta1.NewArrayOrString("fourth-value")},
					{Name: "first-task-fifth-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
	}, {
		name: "single parameter in workspace subpath",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
				Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
					{
						Name:      "first-workspace",
						Workspace: "first-workspace",
						SubPath:   "$(params.second-param)",
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeString, Default: v1beta1.NewArrayOrString("default-value")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
				Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
					{
						Name:      "first-workspace",
						Workspace: "first-workspace",
						SubPath:   "second-value",
					},
				},
			}},
		},
	}, {
		name: "object parameter in workspace subpath",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.myobject.key1)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
				Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
					{
						Name:      "first-workspace",
						Workspace: "first-workspace",
						SubPath:   "$(params.myobject.key2)",
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "myobject", Value: *v1beta1.NewObject(map[string]string{
			"key1": "foo",
			"key2": "bar",
		})}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{
					Name: "myobject",
					Type: v1beta1.ParamTypeObject,
					Properties: map[string]v1beta1.PropertySpec{
						"key1": {Type: "string"},
						"key2": {Type: "string"},
					},
					Default: v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("foo")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
				Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
					{
						Name:      "first-workspace",
						Workspace: "first-workspace",
						SubPath:   "bar",
					},
				},
			}},
		},
	},
	} {
		ctx := context.Background()
		if tt.alpha {
			cfg := config.FromContextOrDefaults(ctx)
			cfg.FeatureFlags = &config.FeatureFlags{EnableAPIFields: "alpha"}
			ctx = config.ToContext(ctx, cfg)
		}
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := &v1beta1.PipelineRun{
				Spec: v1beta1.PipelineRunSpec{
					Params: tt.params,
				},
			}
			got := ApplyParameters(ctx, &tt.original, run)
			if d := cmp.Diff(&tt.expected, got); d != "" {
				t.Errorf("ApplyParameters() got diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyParameters_ArrayIndexing(t *testing.T) {
	ctx := context.Background()
	cfg := config.FromContextOrDefaults(ctx)
	cfg.FeatureFlags.EnableAPIFields = config.AlphaAPIFields
	ctx = config.ToContext(ctx, cfg)
	for _, tt := range []struct {
		name     string
		original v1beta1.PipelineSpec
		params   []v1beta1.Param
		expected v1beta1.PipelineSpec
	}{{
		name: "single parameter",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param[1])")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param[0])")},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value", "second-value-again")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("default-value-again")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("second-value")},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
	}, {
		name: "single parameter with when expression",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "$(params.first-param[1])",
					Operator: selection.In,
					Values:   []string{"$(params.second-param[0])"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value", "second-value-again")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeString},
			},
			Tasks: []v1beta1.PipelineTask{{
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "default-value-again",
					Operator: selection.In,
					Values:   []string{"second-value"},
				}},
			}},
		},
	}, {
		name: "pipeline parameter nested inside task parameter",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.$(params.first-param[0]))")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.$(params.second-param[1]))")},
				},
			}},
		},
		params: nil, // no parameter values.
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.default-value)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("$(input.workspace.default-value-again)")},
				},
			}},
		},
	}, {
		name: "array parameter",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default", "array", "value")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("firstelement", "$(params.first-param)")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("firstelement", "$(params.second-param[0])")},
				},
			}},
		},
		params: []v1beta1.Param{
			{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value", "array")},
		},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default", "array", "value")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("firstelement", "default", "array", "value")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("firstelement", "second-value")},
				},
			}},
		},
	}, {
		name: "parameter evaluation with final tasks",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param[0])")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param[1])")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "$(params.first-param[0])",
					Operator: selection.In,
					Values:   []string{"$(params.second-param[1])"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value", "second-value-again")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("second-value-again")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "default-value",
					Operator: selection.In,
					Values:   []string{"second-value-again"},
				}},
			}},
		},
	}, {
		name: "parameter evaluation with both tasks and final tasks",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param[0])")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param[1])")},
				},
			}},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param[0])")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("$(params.second-param[1])")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "$(params.first-param[0])",
					Operator: selection.In,
					Values:   []string{"$(params.second-param[1])"},
				}},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value", "second-value-again")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("second-value-again")},
				},
			}},
			Finally: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "final-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "final-task-second-param", Value: *v1beta1.NewArrayOrString("second-value-again")},
				},
				WhenExpressions: v1beta1.WhenExpressions{{
					Input:    "default-value",
					Operator: selection.In,
					Values:   []string{"second-value-again"},
				}},
			}},
		},
	}, {
		name: "parameter references with bracket notation and special characters",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first.param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second/param", Type: v1beta1.ParamTypeArray},
				{Name: "third.param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "fourth/param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString(`$(params["first.param"][0])`)},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString(`$(params["second/param"][0])`)},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString(`$(params['third.param'][1])`)},
					{Name: "first-task-fourth-param", Value: *v1beta1.NewArrayOrString(`$(params['fourth/param'][1])`)},
					{Name: "first-task-fifth-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
		params: []v1beta1.Param{
			{Name: "second/param", Value: *v1beta1.NewArrayOrString("second-value", "second-value-again")},
			{Name: "fourth/param", Value: *v1beta1.NewArrayOrString("fourth-value", "fourth-value-again")},
		},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first.param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second/param", Type: v1beta1.ParamTypeArray},
				{Name: "third.param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "fourth/param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("second-value")},
					{Name: "first-task-third-param", Value: *v1beta1.NewArrayOrString("default-value-again")},
					{Name: "first-task-fourth-param", Value: *v1beta1.NewArrayOrString("fourth-value-again")},
					{Name: "first-task-fifth-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
			}},
		},
	}, {
		name: "single parameter in workspace subpath",
		original: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("$(params.first-param[0])")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
				Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
					{
						Name:      "first-workspace",
						Workspace: "first-workspace",
						SubPath:   "$(params.second-param[1])",
					},
				},
			}},
		},
		params: []v1beta1.Param{{Name: "second-param", Value: *v1beta1.NewArrayOrString("second-value", "second-value-again")}},
		expected: v1beta1.PipelineSpec{
			Params: []v1beta1.ParamSpec{
				{Name: "first-param", Type: v1beta1.ParamTypeArray, Default: v1beta1.NewArrayOrString("default-value", "default-value-again")},
				{Name: "second-param", Type: v1beta1.ParamTypeArray},
			},
			Tasks: []v1beta1.PipelineTask{{
				Params: []v1beta1.Param{
					{Name: "first-task-first-param", Value: *v1beta1.NewArrayOrString("default-value")},
					{Name: "first-task-second-param", Value: *v1beta1.NewArrayOrString("static value")},
				},
				Workspaces: []v1beta1.WorkspacePipelineTaskBinding{
					{
						Name:      "first-workspace",
						Workspace: "first-workspace",
						SubPath:   "second-value-again",
					},
				},
			}},
		},
	},
	} {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := &v1beta1.PipelineRun{
				Spec: v1beta1.PipelineRunSpec{
					Params: tt.params,
				},
			}
			got := ApplyParameters(ctx, &tt.original, run)
			if d := cmp.Diff(&tt.expected, got); d != "" {
				t.Errorf("ApplyParameters() got diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyTaskResults_MinimalExpression(t *testing.T) {
	for _, tt := range []struct {
		name               string
		targets            PipelineRunState
		resolvedResultRefs ResolvedResultRefs
		want               PipelineRunState
	}{{
		name: "Test result substitution on minimal variable substitution expression - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("aResultValue"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "a.Result",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"])`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("aResultValue"),
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution on minimal variable substitution expression - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "a.Result",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"][1])`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("arrayResultValueTwo"),
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution out of bound - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "a.Result",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"][3])`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name: "bParam",
					// index validation is done in ResolveResultRefs() before ApplyTaskResults()
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"][3])`),
				}},
			},
		}},
	}, {
		name: "Test array result substitution on minimal variable substitution expression - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "a.Result",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name: "bParam",
					Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeArray,
						ArrayVal: []string{`$(tasks.aTask.results["a.Result"][*])`},
					},
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
				}},
			},
		}},
	}, {
		name: "Test object result as a whole substitution - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewObject(map[string]string{
				"key1": "val1",
				"key2": "val2",
			}),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "resultName",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results.resultName[*])`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name: "bParam",
					// index validation is done in ResolveResultRefs() before ApplyTaskResults()
					Value: *v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				}},
			},
		}},
	}, {
		name: "Test object result element substitution - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewObject(map[string]string{
				"key1": "val1",
				"key2": "val2",
			}),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "resultName",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results.resultName.key1)`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name: "bParam",
					// index validation is done in ResolveResultRefs() before ApplyTaskResults()
					Value: *v1beta1.NewArrayOrString("val1"),
				}},
			},
		}},
	}, {
		name: "Test result substitution on minimal variable substitution expression - matrix",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("aResultValue"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "a.Result",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"])`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("aResultValue"),
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution on minimal variable substitution expression - matrix",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "a.Result",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"][1])`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("arrayResultValueTwo"),
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution out of bound - matrix",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "a.Result",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"][3])`),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString(`$(tasks.aTask.results["a.Result"][3])`),
				}},
			},
		}},
	}, {
		name: "Test array result substitution on minimal variable substitution expression - when expressions",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					// Note that Input doesn't support array replacement.
					Input:    "anInput",
					Operator: selection.In,
					Values:   []string{"$(tasks.aTask.results.aResult[*])"},
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "anInput",
					Operator: selection.In,
					Values:   []string{"arrayResultValueOne", "arrayResultValueTwo"},
				}},
			},
		}},
	}, {
		name: "Test result substitution on minimal variable substitution expression - when expressions",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("aResultValue"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "$(tasks.aTask.results.aResult)",
					Operator: selection.In,
					Values:   []string{"$(tasks.aTask.results.aResult)"},
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "aResultValue",
					Operator: selection.In,
					Values:   []string{"aResultValue"},
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution on minimal variable substitution expression - when expressions",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "$(tasks.aTask.results.aResult[1])",
					Operator: selection.In,
					Values:   []string{"$(tasks.aTask.results.aResult[0])"},
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "arrayResultValueTwo",
					Operator: selection.In,
					Values:   []string{"arrayResultValueOne"},
				}},
			},
		}},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			ApplyTaskResults(tt.targets, tt.resolvedResultRefs)
			if d := cmp.Diff(tt.want, tt.targets); d != "" {
				t.Fatalf("ApplyTaskResults() %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyTaskResults_EmbeddedExpression(t *testing.T) {
	for _, tt := range []struct {
		name               string
		targets            PipelineRunState
		resolvedResultRefs ResolvedResultRefs
		want               PipelineRunState
	}{{
		name: "Test result substitution on embedded variable substitution expression - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("aResultValue"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> $(tasks.aTask.results.aResult)"),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> aResultValue"),
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution on embedded variable substitution expression - params",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> $(tasks.aTask.results.aResult[0])"),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Params: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> arrayResultValueOne"),
				}},
			},
		}},
	}, {
		name: "Test result substitution on embedded variable substitution expression - matrix",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("aResultValue"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> $(tasks.aTask.results.aResult)"),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> aResultValue"),
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution on embedded variable substitution expression - matrix",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> $(tasks.aTask.results.aResult[0])"),
				}},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				Matrix: []v1beta1.Param{{
					Name:  "bParam",
					Value: *v1beta1.NewArrayOrString("Result value --> arrayResultValueOne"),
				}},
			},
		}},
	}, {
		name: "Test result substitution on embedded variable substitution expression - when expressions",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("aResultValue"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{
					{
						Input:    "Result value --> $(tasks.aTask.results.aResult)",
						Operator: selection.In,
						Values:   []string{"Result value --> $(tasks.aTask.results.aResult)"},
					},
				},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "Result value --> aResultValue",
					Operator: selection.In,
					Values:   []string{"Result value --> aResultValue"},
				}},
			},
		}},
	}, {
		name: "Test array indexing result substitution on embedded variable substitution expression - when expressions",
		resolvedResultRefs: ResolvedResultRefs{{
			Value: *v1beta1.NewArrayOrString("arrayResultValueOne", "arrayResultValueTwo"),
			ResultReference: v1beta1.ResultRef{
				PipelineTask: "aTask",
				Result:       "aResult",
			},
			FromTaskRun: "aTaskRun",
		}},
		targets: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{
					{
						Input:    "Result value --> $(tasks.aTask.results.aResult[1])",
						Operator: selection.In,
						Values:   []string{"Result value --> $(tasks.aTask.results.aResult[0])"},
					},
				},
			},
		}},
		want: PipelineRunState{{
			PipelineTask: &v1beta1.PipelineTask{
				Name:    "bTask",
				TaskRef: &v1beta1.TaskRef{Name: "bTask"},
				WhenExpressions: []v1beta1.WhenExpression{{
					Input:    "Result value --> arrayResultValueTwo",
					Operator: selection.In,
					Values:   []string{"Result value --> arrayResultValueOne"},
				}},
			},
		}},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			ApplyTaskResults(tt.targets, tt.resolvedResultRefs)
			if d := cmp.Diff(tt.want, tt.targets); d != "" {
				t.Fatalf("ApplyTaskResults() %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestContext(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		description string
		pr          *v1beta1.PipelineRun
		original    v1beta1.Param
		expected    v1beta1.Param
	}{{
		description: "context.pipeline.name defined",
		pr: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "name"},
		},
		original: v1beta1.Param{Value: *v1beta1.NewArrayOrString("$(context.pipeline.name)-1")},
		expected: v1beta1.Param{Value: *v1beta1.NewArrayOrString("test-pipeline-1")},
	}, {
		description: "context.pipelineRun.name defined",
		pr: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "name"},
		},
		original: v1beta1.Param{Value: *v1beta1.NewArrayOrString("$(context.pipelineRun.name)-1")},
		expected: v1beta1.Param{Value: *v1beta1.NewArrayOrString("name-1")},
	}, {
		description: "context.pipelineRun.name undefined",
		pr: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: ""},
		},
		original: v1beta1.Param{Value: *v1beta1.NewArrayOrString("$(context.pipelineRun.name)-1")},
		expected: v1beta1.Param{Value: *v1beta1.NewArrayOrString("-1")},
	}, {
		description: "context.pipelineRun.namespace defined",
		pr: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Namespace: "namespace"},
		},
		original: v1beta1.Param{Value: *v1beta1.NewArrayOrString("$(context.pipelineRun.namespace)-1")},
		expected: v1beta1.Param{Value: *v1beta1.NewArrayOrString("namespace-1")},
	}, {
		description: "context.pipelineRun.namespace undefined",
		pr: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Namespace: ""},
		},
		original: v1beta1.Param{Value: *v1beta1.NewArrayOrString("$(context.pipelineRun.namespace)-1")},
		expected: v1beta1.Param{Value: *v1beta1.NewArrayOrString("-1")},
	}, {
		description: "context.pipelineRun.uid defined",
		pr: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{UID: "UID"},
		},
		original: v1beta1.Param{Value: *v1beta1.NewArrayOrString("$(context.pipelineRun.uid)-1")},
		expected: v1beta1.Param{Value: *v1beta1.NewArrayOrString("UID-1")},
	}, {
		description: "context.pipelineRun.uid undefined",
		pr: &v1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{UID: ""},
		},
		original: v1beta1.Param{Value: *v1beta1.NewArrayOrString("$(context.pipelineRun.uid)-1")},
		expected: v1beta1.Param{Value: *v1beta1.NewArrayOrString("-1")},
	}} {
		t.Run(tc.description, func(t *testing.T) {
			orig := &v1beta1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline"},
				Spec: v1beta1.PipelineSpec{
					Tasks: []v1beta1.PipelineTask{{
						Params: []v1beta1.Param{tc.original},
						Matrix: []v1beta1.Param{tc.original},
					}},
				},
			}
			got := ApplyContexts(ctx, &orig.Spec, orig.Name, tc.pr)
			if d := cmp.Diff(tc.expected, got.Tasks[0].Params[0]); d != "" {
				t.Errorf(diff.PrintWantGot(d))
			}
			if d := cmp.Diff(tc.expected, got.Tasks[0].Matrix[0]); d != "" {
				t.Errorf(diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyPipelineTaskContexts(t *testing.T) {
	for _, tc := range []struct {
		description string
		pt          v1beta1.PipelineTask
		want        v1beta1.PipelineTask
	}{{
		description: "context retries replacement",
		pt: v1beta1.PipelineTask{
			Retries: 5,
			Params: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("$(context.pipelineTask.retries)"),
			}},
			Matrix: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("$(context.pipelineTask.retries)"),
			}},
		},
		want: v1beta1.PipelineTask{
			Retries: 5,
			Params: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("5"),
			}},
			Matrix: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("5"),
			}},
		},
	}, {
		description: "context retries replacement with no defined retries",
		pt: v1beta1.PipelineTask{
			Params: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("$(context.pipelineTask.retries)"),
			}},
			Matrix: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("$(context.pipelineTask.retries)"),
			}},
		},
		want: v1beta1.PipelineTask{
			Params: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("0"),
			}},
			Matrix: []v1beta1.Param{{
				Name:  "retries",
				Value: *v1beta1.NewArrayOrString("0"),
			}},
		},
	}} {
		t.Run(tc.description, func(t *testing.T) {
			got := ApplyPipelineTaskContexts(&tc.pt)
			if d := cmp.Diff(&tc.want, got); d != "" {
				t.Errorf(diff.PrintWantGot(d))
			}
		})
	}
}

func TestApplyWorkspaces(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		description         string
		declarations        []v1beta1.PipelineWorkspaceDeclaration
		bindings            []v1beta1.WorkspaceBinding
		variableUsage       string
		expectedReplacement string
	}{{
		description: "workspace declared and bound",
		declarations: []v1beta1.PipelineWorkspaceDeclaration{{
			Name: "foo",
		}},
		bindings: []v1beta1.WorkspaceBinding{{
			Name: "foo",
		}},
		variableUsage:       "$(workspaces.foo.bound)",
		expectedReplacement: "true",
	}, {
		description: "workspace declared not bound",
		declarations: []v1beta1.PipelineWorkspaceDeclaration{{
			Name:     "foo",
			Optional: true,
		}},
		bindings:            []v1beta1.WorkspaceBinding{},
		variableUsage:       "$(workspaces.foo.bound)",
		expectedReplacement: "false",
	}} {
		t.Run(tc.description, func(t *testing.T) {
			p1 := v1beta1.PipelineSpec{
				Tasks: []v1beta1.PipelineTask{{
					Params: []v1beta1.Param{{Value: *v1beta1.NewArrayOrString(tc.variableUsage)}},
				}},
				Workspaces: tc.declarations,
			}
			pr := &v1beta1.PipelineRun{
				Spec: v1beta1.PipelineRunSpec{
					PipelineRef: &v1beta1.PipelineRef{
						Name: "test-pipeline",
					},
					Workspaces: tc.bindings,
				},
			}
			p2 := ApplyWorkspaces(ctx, &p1, pr)
			str := p2.Tasks[0].Params[0].Value.StringVal
			if str != tc.expectedReplacement {
				t.Errorf("expected %q, received %q", tc.expectedReplacement, str)
			}
		})
	}
}

func TestApplyTaskResultsToPipelineResults(t *testing.T) {
	for _, tc := range []struct {
		description     string
		results         []v1beta1.PipelineResult
		taskResults     map[string][]v1beta1.TaskRunResult
		runResults      map[string][]v1alpha1.RunResult
		expectedResults []v1beta1.PipelineRunResult
		expectedError   error
	}{{
		description: "non-reference-results",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("resultName"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name:  "foo",
					Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
				},
			},
		},
		expectedResults: nil,
	}, {
		description: "array-index-out-of-bound",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo[4])"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name:  "foo",
					Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
				},
			},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [pipeline-result-1], the referred results don't exist"),
	}, {
		description: "object-reference-key-not-exist",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo.key3)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name: "foo",
					Value: *v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [pipeline-result-1], the referred results don't exist"),
	}, {
		description: "object-results-resultname-not-exist",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.bar.key1)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name: "foo",
					Value: *v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [pipeline-result-1], the referred results don't exist"),
	}, {
		description: "apply-array-results",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo[*])"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name:  "foo",
					Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
				},
			},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
		}},
	}, {
		description: "apply-array-indexing-results",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo[1])"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name:  "foo",
					Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
				},
			},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("rae"),
		}},
	}, {
		description: "apply-object-results",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo[*])"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name: "foo",
					Value: *v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name: "pipeline-result-1",
			Value: *v1beta1.NewObject(map[string]string{
				"key1": "val1",
				"key2": "val2",
			}),
		}},
	}, {
		description: "object-results-from-array-indexing-and-object-element",
		results: []v1beta1.PipelineResult{{
			Name: "pipeline-result-1",
			Value: *v1beta1.NewObject(map[string]string{
				"pkey1": "$(tasks.pt1.results.foo.key1)",
				"pkey2": "$(tasks.pt2.results.bar[1])",
			}),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name: "foo",
					Value: *v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			"pt2": {
				{
					Name:  "bar",
					Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
				},
			},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name: "pipeline-result-1",
			Value: *v1beta1.NewObject(map[string]string{
				"pkey1": "val1",
				"pkey2": "rae",
			}),
		}},
	}, {
		description: "array-results-from-array-indexing-and-object-element",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo.key1)", "$(tasks.pt2.results.bar[1])"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name: "foo",
					Value: *v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
			"pt2": {
				{
					Name:  "bar",
					Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
				},
			},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("val1", "rae"),
		}},
	}, {
		description: "apply-object-element",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo.key1)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name: "foo",
					Value: *v1beta1.NewObject(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}),
				},
			},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("val1"),
		}},
	}, {
		description: "multiple-array-results-multiple-successful-tasks ",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo[*])"),
		}, {
			Name:  "pipeline-result-2",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt2.results.bar[*])"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name:  "foo",
					Value: *v1beta1.NewArrayOrString("do", "rae"),
				},
			},
			"pt2": {
				{
					Name:  "bar",
					Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
				},
			},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("do", "rae"),
		}, {
			Name:  "pipeline-result-2",
			Value: *v1beta1.NewArrayOrString("do", "rae", "mi"),
		}},
	}, {
		description: "no-pipeline-results-no-returned-results",
		results:     []v1beta1.PipelineResult{},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {{
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		expectedResults: nil,
	}, {
		description: "invalid-result-variable-no-returned-result",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1_results.foo)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {{
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "no-taskrun-results-no-returned-results",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "invalid-taskrun-name-no-returned-result",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"definitely-not-pt1": {{
				Name:  "foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		expectedResults: nil,
	}, {
		description: "invalid-result-name-no-returned-result",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {{
				Name:  "definitely-not-foo",
				Value: *v1beta1.NewArrayOrString("bar"),
			}},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "unsuccessful-taskrun-no-returned-result",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo)"),
		}},
		taskResults:     map[string][]v1beta1.TaskRunResult{},
		expectedResults: nil,
	}, {
		description: "mixed-success-tasks-some-returned-results",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo)"),
		}, {
			Name:  "bar",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt2.results.bar)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt2": {{
				Name:  "bar",
				Value: *v1beta1.NewArrayOrString("rae"),
			}},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "bar",
			Value: *v1beta1.NewArrayOrString("rae"),
		}},
		expectedError: fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "multiple-results-multiple-successful-tasks ",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo)"),
		}, {
			Name:  "pipeline-result-2",
			Value: *v1beta1.NewArrayOrString("$(tasks.pt1.results.foo), $(tasks.pt2.results.baz), $(tasks.pt1.results.bar), $(tasks.pt2.results.baz), $(tasks.pt1.results.foo)"),
		}},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"pt1": {
				{
					Name:  "foo",
					Value: *v1beta1.NewArrayOrString("do"),
				}, {
					Name:  "bar",
					Value: *v1beta1.NewArrayOrString("mi"),
				},
			},
			"pt2": {{
				Name:  "baz",
				Value: *v1beta1.NewArrayOrString("rae"),
			}},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("do"),
		}, {
			Name:  "pipeline-result-2",
			Value: *v1beta1.NewArrayOrString("do, rae, mi, rae, do"),
		}},
	}, {
		description: "no-run-results-no-returned-results",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.customtask.results.foo)"),
		}},
		runResults:      map[string][]v1alpha1.RunResult{},
		expectedResults: nil,
	}, {
		description: "wrong-customtask-name-no-returned-result",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.customtask.results.foo)"),
		}},
		runResults: map[string][]v1alpha1.RunResult{
			"differentcustomtask": {{
				Name:  "foo",
				Value: "bar",
			}},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "right-customtask-name-wrong-result-name-no-returned-result",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.customtask.results.foo)"),
		}},
		runResults: map[string][]v1alpha1.RunResult{
			"customtask": {{
				Name:  "notfoo",
				Value: "bar",
			}},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "unsuccessful-run-no-returned-result",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.customtask.results.foo)"),
		}},
		runResults: map[string][]v1alpha1.RunResult{
			"customtask": {},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "wrong-result-reference-expression",
		results: []v1beta1.PipelineResult{{
			Name:  "foo",
			Value: *v1beta1.NewArrayOrString("$(tasks.task.results.foo.foo.foo)"),
		}},
		runResults: map[string][]v1alpha1.RunResult{
			"customtask": {},
		},
		expectedResults: nil,
		expectedError:   fmt.Errorf("invalid pipelineresults [foo], the referred results don't exist"),
	}, {
		description: "multiple-results-custom-and-normal-tasks",
		results: []v1beta1.PipelineResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("$(tasks.customtask.results.foo)"),
		}, {
			Name:  "pipeline-result-2",
			Value: *v1beta1.NewArrayOrString("$(tasks.customtask.results.foo), $(tasks.normaltask.results.baz), $(tasks.customtask.results.bar), $(tasks.normaltask.results.baz), $(tasks.customtask.results.foo)"),
		}},
		runResults: map[string][]v1alpha1.RunResult{
			"customtask": {
				{
					Name:  "foo",
					Value: "do",
				}, {
					Name:  "bar",
					Value: "mi",
				},
			},
		},
		taskResults: map[string][]v1beta1.TaskRunResult{
			"normaltask": {{
				Name:  "baz",
				Value: *v1beta1.NewArrayOrString("rae"),
			}},
		},
		expectedResults: []v1beta1.PipelineRunResult{{
			Name:  "pipeline-result-1",
			Value: *v1beta1.NewArrayOrString("do"),
		}, {
			Name:  "pipeline-result-2",
			Value: *v1beta1.NewArrayOrString("do, rae, mi, rae, do"),
		}},
	}} {
		t.Run(tc.description, func(t *testing.T) {
			received, err := ApplyTaskResultsToPipelineResults(tc.results, tc.taskResults, tc.runResults)
			if tc.expectedError != nil {
				if d := cmp.Diff(tc.expectedError.Error(), err.Error()); d != "" {
					t.Errorf("ApplyTaskResultsToPipelineResults() errors diff %s", diff.PrintWantGot(d))
				}
			}
			if d := cmp.Diff(tc.expectedResults, received); d != "" {
				t.Errorf(diff.PrintWantGot(d))
			}
		})
	}

}

func TestApplyTaskRunContext(t *testing.T) {
	r := map[string]string{
		"tasks.task1.status": "succeeded",
		"tasks.task3.status": "none",
	}
	state := PipelineRunState{{
		PipelineTask: &v1beta1.PipelineTask{
			Name:    "task4",
			TaskRef: &v1beta1.TaskRef{Name: "task"},
			Params: []v1beta1.Param{{
				Name:  "task1",
				Value: *v1beta1.NewArrayOrString("$(tasks.task1.status)"),
			}, {
				Name:  "task3",
				Value: *v1beta1.NewArrayOrString("$(tasks.task3.status)"),
			}},
			WhenExpressions: v1beta1.WhenExpressions{{
				Input:    "$(tasks.task1.status)",
				Operator: selection.In,
				Values:   []string{"$(tasks.task3.status)"},
			}},
		},
	}}
	expectedState := PipelineRunState{{
		PipelineTask: &v1beta1.PipelineTask{
			Name:    "task4",
			TaskRef: &v1beta1.TaskRef{Name: "task"},
			Params: []v1beta1.Param{{
				Name:  "task1",
				Value: *v1beta1.NewArrayOrString("succeeded"),
			}, {
				Name:  "task3",
				Value: *v1beta1.NewArrayOrString("none"),
			}},
			WhenExpressions: v1beta1.WhenExpressions{{
				Input:    "succeeded",
				Operator: selection.In,
				Values:   []string{"none"},
			}},
		},
	}}
	ApplyPipelineTaskStateContext(state, r)
	if d := cmp.Diff(expectedState, state); d != "" {
		t.Fatalf("ApplyTaskRunContext() %s", diff.PrintWantGot(d))
	}
}
