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

from google.cloud import bigquery, firestore, storage
import pandas as pd
import os
from dotenv import load_dotenv
import time

# Load environment variables
load_dotenv()

# Get environment variables
PROJECT_ID = os.getenv('project_id')
DATASET = os.getenv('bq_dataset')
TABLE = os.getenv('bq_table')
ENRICHED_TABLE = os.getenv('bq_enriched_table')
FIRESTORE_COLLECTION = os.getenv('firestore_collection')

def fetch_all_products_from_bigquery():
    """Fetch all products from BigQuery."""
    client = bigquery.Client(project=PROJECT_ID)
    
    query = f"""
    SELECT *
    FROM `{PROJECT_ID}.{DATASET}.{TABLE}`
    ORDER BY id
    """
    
    print("Fetching all products from BigQuery...")
    df = client.query(query).to_dataframe()
    print(f"Fetched {len(df)} products from BigQuery")
    return df

def fetch_processed_products_from_firestore():
    """Fetch all successfully processed products from Firestore."""
    db = firestore.Client(project=PROJECT_ID)
    collection = db.collection(FIRESTORE_COLLECTION)
    
    print("Fetching processed products from Firestore...")
    # Query only completed products
    docs = collection.where(filter=firestore.FieldFilter('status', '==', 'completed')).stream()
    
    processed_products = []
    for doc in docs:
        try:
            product_id = int(doc.id)  # Convert string ID to int
            data = doc.to_dict()
            print(f"\nDebug - Firestore document for product {product_id}:")
            print(f"Data: {data}")
            
            # Extract data from the nested product_data if it exists
            product_data = data.get('product_data', {})
            
            # Create base product info
            product_info = {
                'id': product_id,
                'image_uri': data.get('image_uri'),
                'description': data.get('description'),
                'completed_at': data.get('completed_at'),
                'status': data.get('status'),
                'started_at': data.get('started_at'),
                'updated_at': data.get('updated_at')
            }
            
            # Add all product_data fields except 'id' which we already have
            if product_data:
                product_data.pop('id', None)  # Remove id from product_data if it exists
                product_info.update(product_data)
            
            processed_products.append(product_info)
        except Exception as e:
            print(f"Error processing document {doc.id}: {str(e)}")
            continue
    
    print(f"\nFetched {len(processed_products)} processed products from Firestore")
    if processed_products:
        print("\nSample of first processed product:")
        print(processed_products[0])
    
    return pd.DataFrame(processed_products) if processed_products else pd.DataFrame(columns=['id', 'image_uri', 'description', 'completed_at', 'status', 'started_at', 'updated_at'])

def fetch_failed_products_from_firestore():
    """Fetch all permanently failed products from Firestore."""
    db = firestore.Client(project=PROJECT_ID)
    collection = db.collection(FIRESTORE_COLLECTION)
    
    print("\nFetching failed products from Firestore...")
    # Query permanently failed products
    docs = collection.where(filter=firestore.FieldFilter('status', '==', 'permanently_failed')).stream()
    
    failed_products = []
    for doc in docs:
        try:
            product_id = int(doc.id)
            data = doc.to_dict()
            product_data = data.get('product_data', {})
            
            # Create base product info
            product_info = {
                'id': product_id,
                'error_message': data.get('error_message'),
                'retry_count': data.get('retry_count'),
                'failed_at': data.get('failed_at'),
                'status': data.get('status'),
                'started_at': data.get('started_at'),
                'updated_at': data.get('updated_at')
            }
            
            # Add all product_data fields except 'id'
            if product_data:
                product_data.pop('id', None)  # Remove id from product_data if it exists
                product_info.update(product_data)
            
            failed_products.append(product_info)
        except Exception as e:
            print(f"Error processing document {doc.id}: {str(e)}")
            continue
    
    print(f"Fetched {len(failed_products)} failed products from Firestore")
    return pd.DataFrame(failed_products) if failed_products else pd.DataFrame(columns=['id', 'error_message', 'retry_count', 'failed_at', 'status', 'started_at', 'updated_at'])

def upload_to_gcs(df, filename):
    """Upload DataFrame as CSV to Google Cloud Storage."""
    storage_client = storage.Client()
    bucket = storage_client.bucket(os.getenv('psearch_img_bucket'))
    
    # Save DataFrame to a temporary CSV file
    temp_filename = f"temp_{filename}"
    df.to_csv(temp_filename, index=False)
    
    # Upload to GCS
    blob = bucket.blob(f"exports/{filename}")
    blob.upload_from_filename(temp_filename)
    
    # Clean up temporary file
    os.remove(temp_filename)
    
    print(f"Uploaded consolidated results to gs://{os.getenv('psearch_img_bucket')}/exports/{filename}")

def write_to_bigquery(df, table_name):
    """Write DataFrame to BigQuery table."""
    client = bigquery.Client(project=PROJECT_ID)
    table_id = f"{PROJECT_ID}.{DATASET}.{table_name}"
    
    print(f"Writing {len(df)} rows to BigQuery table: {table_id}")
    
    # Configure the job
    job_config = bigquery.LoadJobConfig(
        write_disposition="WRITE_TRUNCATE",  # Overwrite the table if it exists
    )
    
    job = client.load_table_from_dataframe(
        df, table_id, job_config=job_config
    )
    
    # Wait for the job to complete
    job.result()
    print(f"✓ Successfully wrote {len(df)} rows to {table_id}")

def consolidate_results():
    """Consolidate results from BigQuery and Firestore."""
    # Fetch all data
    bq_df = fetch_all_products_from_bigquery()
    processed_df = fetch_processed_products_from_firestore()
    failed_df = fetch_failed_products_from_firestore()
    
    print("\nDebug - Processed DataFrame Info:")
    print(processed_df.info())
    if not processed_df.empty:
        print("\nSample of processed data:")
        print(processed_df.head())
    
    # Initialize merged DataFrame with BigQuery data
    merged_df = bq_df.copy()
    
    # Add columns for processing data with default values
    merged_df['image_uri'] = None
    merged_df['description'] = None
    merged_df['completed_at'] = None
    merged_df['error_message'] = None
    merged_df['retry_count'] = 0
    merged_df['failed_at'] = None
    merged_df['started_at'] = None
    merged_df['updated_at'] = None
    merged_df['processing_status'] = 'pending'
    
    # Update processed products if any exist
    if not processed_df.empty:
        print("\nUpdating processed products...")
        processed_columns = ['image_uri', 'description', 'completed_at', 'started_at', 'updated_at']
        
        for _, row in processed_df.iterrows():
            product_id = row['id']
            mask = merged_df['id'] == product_id
            print(f"\nUpdating product {product_id}:")
            print(f"Original data: {merged_df.loc[mask, processed_columns].to_dict('records')}")
            print(f"New data: {row[processed_columns].to_dict()}")
            
            # Update all relevant columns
            for col in processed_columns:
                if col in row and pd.notna(row[col]):
                    merged_df.loc[mask, col] = row[col]
            
            merged_df.loc[mask, 'processing_status'] = 'completed'
    
    # Update failed products if any exist
    if not failed_df.empty:
        print("\nUpdating failed products...")
        failed_columns = ['error_message', 'retry_count', 'failed_at', 'started_at', 'updated_at']
        
        for _, row in failed_df.iterrows():
            product_id = row['id']
            mask = merged_df['id'] == product_id
            print(f"\nUpdating failed product {product_id}:")
            print(f"Original data: {merged_df.loc[mask, failed_columns].to_dict('records')}")
            print(f"New data: {row[failed_columns].to_dict()}")
            
            # Update all relevant columns
            for col in failed_columns:
                if col in row and pd.notna(row[col]):
                    merged_df.loc[mask, col] = row[col]
            
            merged_df.loc[mask, 'processing_status'] = 'failed'
    
    # Print sample of final data
    print("\nSample of final merged data:")
    sample_completed = merged_df[merged_df['processing_status'] == 'completed'].head()
    if not sample_completed.empty:
        print("\nCompleted products sample:")
        display_columns = ['id', 'name', 'brand', 'image_uri', 'description', 'completed_at', 'processing_status']
        print(sample_completed[display_columns])
    
    # Generate timestamp for filename
    timestamp = int(time.time())
    filename = f"consolidated_products_{timestamp}.csv"
    
    # Upload to GCS
    upload_to_gcs(merged_df, filename)
    
    # Write to BigQuery
    write_to_bigquery(merged_df, ENRICHED_TABLE)
    
    # Print summary
    print("\nProcessing Summary:")
    print(f"Total Products: {len(merged_df)}")
    print(f"Completed: {len(merged_df[merged_df['processing_status'] == 'completed'])}")
    print(f"Failed: {len(merged_df[merged_df['processing_status'] == 'failed'])}")
    print(f"Pending: {len(merged_df[merged_df['processing_status'] == 'pending'])}")

if __name__ == "__main__":
    consolidate_results() 