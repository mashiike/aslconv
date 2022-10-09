comment  = "Parallel Example."
start_at = state.parallel.LookupCustomerInfo

state "parallel" "LookupCustomerInfo" {
  end = true

  branch {
    start_at = state.task.LookupAddress

    state "task" "LookupAddress" {
      resource = "arn:aws:lambda:us-east-1:123456789012:function:AddressFinder"
      end      = true
    }
  }

  branch {
    start_at = state.task.LookupPhone

    state "task" "LookupPhone" {
      resource = "arn:aws:lambda:us-east-1:123456789012:function:PhoneFinder"
      end      = true
    }
  }
}
