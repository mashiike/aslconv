comment  = "An example of the Amazon States Language using a choice state."
start_at = state.task.FirstState

locals {
    region = "us-east-1"
    aws_account_id = "123456789012"
    default_lambda_function_arn = "arn:aws:lambda:${local.region}:${local.aws_account_id}:function:FUNCTION_NAME"
    on_first_match_lambda_function_arn = "arn:aws:lambda:${local.region}:${local.aws_account_id}:function:OnFirstMatch"
    on_second_match_lambda_function_arn = "arn:aws:lambda:${local.region}:${local.aws_account_id}:function:OnSecondMatch"
}

state "task" "FirstState" {
  resource = local.default_lambda_function_arn
  next     = state.choice.ChoiceState
}

state "choice" "ChoiceState" {
  choices = [
    jsonencode(
      {
        "Variable"      = "$.foo",
        "NumericEquals" = 1,
        "Next"          = state.task.FirstMatchState,
      },
    ),
    jsonencode(
      {
        "Variable"      = "$.foo",
        "NumericEquals" = 2,
        "Next"          = state.task.SecondMatchState,
      },
    ),
  ]
  default = state.fail.DefaultState
}

state "task" "FirstMatchState" {
  resource = local.on_first_match_lambda_function_arn
  next     = state.task.NextState
}

state "task" "SecondMatchState" {
  resource = local.on_second_match_lambda_function_arn
  next     = state.task.NextState
}

state "fail" "DefaultState" {
  error = "DefaultStateError"
  cause = "No Matches!"
}

state "task" "NextState" {
  resource = local.default_lambda_function_arn
  end      = true
}
