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

from google.cloud import firestore
from datetime import datetime
from google.api_core.exceptions import FailedPrecondition

class FirestoreClient:
    def __init__(self, project_id, collection_name):
        self.db = firestore.Client(project=project_id)
        self.collection = self.db.collection(collection_name)
    
    def get_last_processed_id(self):
        """Get the last processed ID from Firestore."""
        doc_ref = self.collection.document('processing_status')
        doc = doc_ref.get()
        
        if doc.exists:
            return doc.to_dict().get('last_processed_id', 0)
        return 0

    def update_last_processed_id(self, last_id):
        """Update the last processed ID in Firestore."""
        doc_ref = self.collection.document('processing_status')
        doc_ref.set({
            'last_processed_id': last_id,
            'last_updated': firestore.SERVER_TIMESTAMP
        })
    
    def is_product_processed(self, product_id):
        """Check if a product has already been processed successfully."""
        doc_ref = self.collection.document(str(product_id))
        doc = doc_ref.get()
        if doc.exists:
            status = doc.to_dict().get('status')
            retry_count = doc.to_dict().get('retry_count', 0)
            # Return True if completed or failed too many times
            return status == 'completed' or retry_count >= 3
        return False
    
    def get_failed_products(self):
        """Get list of products that failed and haven't exceeded retry limit."""
        try:
            # First try with a simple status filter
            failed_docs = (
                self.collection
                .where(filter=firestore.FieldFilter('status', '==', 'failed'))
                .stream()
            )
            
            # Filter retry count in memory
            failed_products = []
            for doc in failed_docs:
                data = doc.to_dict()
                retry_count = data.get('retry_count', 0)
                if retry_count < 3:
                    failed_products.append({
                        'id': int(doc.id),
                        **data
                    })
            
            return failed_products
            
        except FailedPrecondition as e:
            print("\nFirestore index error. Please create the following indexes:")
            print("1. Collection: product_index")
            print("2. Fields to index:")
            print("   - status (Ascending)")
            print("   - retry_count (Ascending)")
            print("   - __name__ (Ascending)")
            print("\nYou can create the index using the Firebase Console or using the following command:")
            print("gcloud firestore indexes composite create --collection-group=product_index --field-config field=status,order=ASCENDING field=retry_count,order=ASCENDING field=__name__,order=ASCENDING")
            print("\nOr visit the following URL to create the index:")
            print("https://console.firebase.google.com/project/psearch-dev/firestore/indexes")
            return []
    
    def start_product_processing(self, product_id, product_data):
        """Mark a product as being processed."""
        doc_ref = self.collection.document(str(product_id))
        doc = doc_ref.get()
        
        data = {
            'status': 'processing',
            'product_data': product_data,
            'started_at': firestore.SERVER_TIMESTAMP,
            'updated_at': firestore.SERVER_TIMESTAMP,
            'retry_count': 0
        }
        
        # If document exists, increment retry count
        if doc.exists:
            current_retry = doc.to_dict().get('retry_count', 0)
            data['retry_count'] = current_retry + 1
        
        doc_ref.set(data)
    
    def complete_product_processing(self, product_id, image_uri, description):
        """Mark a product as completed with generated data."""
        doc_ref = self.collection.document(str(product_id))
        doc_ref.update({
            'status': 'completed',
            'image_uri': image_uri,
            'description': description,
            'completed_at': firestore.SERVER_TIMESTAMP,
            'updated_at': firestore.SERVER_TIMESTAMP
        })
    
    def mark_product_failed(self, product_id, error_message):
        """Mark a product as failed with error details."""
        doc_ref = self.collection.document(str(product_id))
        doc = doc_ref.get()
        
        update_data = {
            'status': 'failed',
            'error_message': error_message,
            'failed_at': firestore.SERVER_TIMESTAMP,
            'updated_at': firestore.SERVER_TIMESTAMP
        }
        
        if doc.exists:
            retry_count = doc.to_dict().get('retry_count', 0)
            update_data['retry_count'] = retry_count
            
            # If we've tried 3 times, mark as permanently failed
            if retry_count >= 3:
                update_data['status'] = 'permanently_failed'
        
        doc_ref.update(update_data) 