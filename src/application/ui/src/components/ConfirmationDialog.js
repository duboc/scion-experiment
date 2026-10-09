/*
 * Copyright 2025 Google LLC
 * 
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 * 
 *     https://www.apache.org/licenses/LICENSE-2.0
 * 
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import React from 'react';
import {
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    Button,
    Typography,
    Box
} from '@mui/material';
import WarningIcon from '@mui/icons-material/Warning';

// ConfirmationDialog component for confirming catalog updates
const ConfirmationDialog = ({
    open,
    contentType,
    onClose,
    onConfirm
}) => {

    // Get dialog content based on the content type
    const getDialogContent = () => {
        switch (contentType) {
            case 'image':
                return {
                    title: 'Update Product Image?',
                    message: 'This will replace the current product image with the AI-enhanced version. This change will be reflected in the product catalog.'
                };
            case 'description':
                return {
                    title: 'Update Product Description?',
                    message: 'This will replace the current product description with the AI-enhanced version. This change will be reflected in the product catalog.'
                };
            case 'specifications':
                return {
                    title: 'Update Product Specifications?',
                    message: 'This will update the product specifications with the AI-enhanced data. This change will be reflected in the product catalog.'
                };
            default:
                return {
                    title: 'Update Product Information?',
                    message: 'This will update the product information with AI-enhanced content. This change will be reflected in the product catalog.'
                };
        }
    };

    const { title, message } = getDialogContent();

    return (
        <Dialog
            open={open}
            onClose={onClose}
            aria-labelledby="confirmation-dialog-title"
            aria-describedby="confirmation-dialog-description"
            PaperProps={{
                sx: { borderRadius: 2 }
            }}
        >
            <DialogTitle id="confirmation-dialog-title" sx={{ pb: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center' }}>
                    <WarningIcon color="warning" sx={{ mr: 1 }} />
                    <Typography variant="h6" component="span">
                        {title}
                    </Typography>
                </Box>
            </DialogTitle>
            <DialogContent>
                <DialogContentText id="confirmation-dialog-description">
                    {message}
                </DialogContentText>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button
                    onClick={onClose}
                    variant="outlined"
                    sx={{
                        color: '#D32F2F',  // Red color from ContentDiff
                        borderColor: '#D32F2F',
                        '&:hover': {
                            borderColor: '#D32F2F',
                            backgroundColor: 'rgba(211, 47, 47, 0.04)'
                        }
                    }}
                >
                    Cancel
                </Button>
                <Button
                    onClick={onConfirm}
                    variant="contained"
                    autoFocus
                    sx={{
                        backgroundColor: '#1E8E3E',  // Green color from ContentDiff
                        '&:hover': {
                            backgroundColor: 'rgba(30, 142, 62, 0.9)'
                        }
                    }}
                >
                    Confirm Update
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ConfirmationDialog;
