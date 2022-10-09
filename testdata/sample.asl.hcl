comment  = "An example of the Amazon States Language using a choice state."
start_at = state.task.FirstState

state "task" "FirstState" {
  resource = "arn:aws:lambda:us-east-1:123456789012:function:FUNCTION_NAME"
  next     = state.choice.ChoiceState
}

state "choice" "ChoiceState" {
  default = state.fail.DefaultState
  choices = ["{\"Variable\":\"$.foo\",\"NumericEquals\":1,\"Next\":\"FirstMatchState\"}", "{\"Variable\":\"$.foo\",\"NumericEquals\":2,\"Next\":\"SecondMatchState\"}"]
}

state "task" "FirstMatchState" {
  resource = "arn:aws:lambda:us-east-1:123456789012:function:OnFirstMatch"
  next     = state.task.NextState
}

state "task" "SecondMatchState" {
  resource = "arn:aws:lambda:us-east-1:123456789012:function:OnSecondMatch"
  next     = state.task.NextState
}

state "fail" "DefaultState" {
  error = "DefaultStateError"
  cause = "No Matches!"
}

state "task" "NextState" {
  resource = "arn:aws:lambda:us-east-1:123456789012:function:FUNCTION_NAME"
  end      = true
}
