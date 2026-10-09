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

import os
import vertexai
from vertexai.preview.vision_models import ImageGenerationModel

def init_imagen(project_id):
    """Initialize Imagen client."""
    vertexai.init(project=project_id, location="us-central1")

def generate_image(row_data, project_id):
    """Generate image based on product data using Vertex AI Imagen."""
    init_imagen(project_id)
    
    # Create a more detailed prompt with specific product attributes
    prompt = f"""Create a professional product image for an e-commerce listing:
Product: {row_data['name']}
Brand: {row_data['brand']}
Category: {row_data['category']} in {row_data['department']} department
Style: Clean, well-lit product photography style with white background
Focus: Show the product clearly with attention to detail and key features"""

    model = ImageGenerationModel.from_pretrained("imagen-3.0-generate-002")
    
    try:
        images = model.generate_images(
            prompt=prompt,
            number_of_images=1,
            language="en",
            aspect_ratio="1:1",
            safety_filter_level="block_some",
            person_generation="allow_adult",
        )
        
        if not images:
            print(f"No images generated for product: {row_data['name']}")
            return None
            
        # Return the first image from the list
        return images[0]
    except Exception as e:
        print(f"Error generating image for product {row_data['name']}: {str(e)}")
        print(f"Prompt used: {prompt}")
        return None 