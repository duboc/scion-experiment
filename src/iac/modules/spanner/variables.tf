# 
# Copyright 2025 Google LLC
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     https://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "The ID of the project where resources will be created"
  type        = string
}

variable "region" {
  description = "The region where resources will be created"
  type        = string
}

variable "service_account_email" {
  description = "The service account email to be granted access to Spanner"
  type        = string
}

variable "processing_units" {
  description = "Number of processing units for the Spanner instance"
  type        = number
  default     = 1000
}

variable "database_name" {
  description = "Name of the Spanner database"
  type        = string
  default     = "products-db"
}

variable "environment" {
  description = "Environment label for resources"
  type        = string
  default     = "development"
}
