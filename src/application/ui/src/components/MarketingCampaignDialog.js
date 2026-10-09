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

import React, { useState, useEffect } from 'react';
import {
    Dialog, DialogTitle, DialogContent, DialogActions,
    Grid, FormControl, InputLabel, Select, MenuItem,
    Button, Typography, Box, CircularProgress, Paper,
    Fade, Zoom, Grow, IconButton
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SendIcon from '@mui/icons-material/Send';
import PersonAddIcon from '@mui/icons-material/PersonAdd';
import CampaignIcon from '@mui/icons-material/Campaign';
import { generateMarketingContent } from '../services/genAiService';
import ProductImage from './ProductImage';

// Content type descriptions
const contentTypeOptions = {
    "product_description": "A compelling product description that highlights key features, benefits, and unique selling points",
    "email_campaign": "An email marketing campaign that promotes the product, encourages click-through, and drives conversions",
    "social_post": "Engaging social media content to promote the product, tailored to the platform's constraints and audience",
    "product_page": "Optimized content for a product detail page with all necessary information for customer conversion",
    "ad_copy": "Compelling advertising copy that drives interest and conversions in limited space",
    "blog_post": "An informative and engaging blog post about the product, its benefits, and use cases",
};

// Tone descriptions
const toneOptions = {
    "professional": "Formal, authoritative, and business-like",
    "casual": "Conversational, friendly, and approachable",
    "luxury": "Sophisticated, exclusive, and premium",
    "technical": "Detailed, precise, and feature-focused",
    "emotional": "Empathetic, personal, and focused on feelings",
    "humorous": "Light-hearted, fun, and entertaining",
};

const MarketingCampaignDialog = ({ open, onClose, product }) => {
    // State for form values
    const [contentType, setContentType] = useState('email_campaign');
    const [tone, setTone] = useState('professional');

    // State for UI
    const [isGenerating, setIsGenerating] = useState(false);
    const [isSelectingUsers, setIsSelectingUsers] = useState(false);
    const [isSending, setIsSending] = useState(false);
    const [generatedContent, setGeneratedContent] = useState(null);
    const [error, setError] = useState(null);

    // Animation states
    const [showDialog, setShowDialog] = useState(false);
    const [showContent, setShowContent] = useState(false);

    // Animation effect when dialog opens
    useEffect(() => {
        if (open) {
            setShowDialog(true);

            // Reset states when dialog opens
            setGeneratedContent(null);
            setError(null);
            setIsGenerating(false);
            setIsSelectingUsers(false);
            setIsSending(false);
        } else {
            setShowDialog(false);
            setShowContent(false);
        }
    }, [open]);

    // Handle content type change
    const handleContentTypeChange = (event) => {
        setContentType(event.target.value);
        // Reset generated content when options change
        setGeneratedContent(null);
    };

    // Handle tone change
    const handleToneChange = (event) => {
        setTone(event.target.value);
        // Reset generated content when options change
        setGeneratedContent(null);
    };

    // Handle generate content
    const handleGenerateContent = async () => {
        setIsGenerating(true);
        setError(null);
        setGeneratedContent(null);

        try {
            const result = await generateMarketingContent(
                product.id,
                product,
                contentType,
                tone
            );

            setGeneratedContent(result.content);
            setShowContent(true);
        } catch (error) {
            console.error('Error generating marketing content:', error);
            setError('Failed to generate marketing content. Please try again.');
        } finally {
            setIsGenerating(false);
        }
    };

    // Mock function for selecting user base
    const handleSelectUserBase = () => {
        setIsSelectingUsers(true);

        // Simulate API call
        setTimeout(() => {
            setIsSelectingUsers(false);
        }, 1500);
    };

    // Mock function for sending campaign
    const handleSendCampaign = () => {
        setIsSending(true);

        // Simulate API call
        setTimeout(() => {
            setIsSending(false);
            onClose();
        }, 1500);
    };

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="md"
            fullWidth
            TransitionComponent={Zoom}
            TransitionProps={{ timeout: 500 }}
            PaperProps={{
                sx: {
                    borderRadius: 2,
                    overflow: 'hidden'
                }
            }}
        >
            <Grow in={showDialog} timeout={500}>
                <Box>
                    <DialogTitle sx={{
                        bgcolor: '#4285f4',
                        color: 'white',
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        p: 2, // Increase padding in the header
                    }}>
                        <Box sx={{ display: 'flex', alignItems: 'center' }}>
                            <CampaignIcon sx={{ mr: 1 }} />
                            <Typography variant="h6">Generate Marketing Campaign</Typography>
                        </Box>
                        <IconButton onClick={onClose} sx={{ color: 'white' }}>
                            <CloseIcon />
                        </IconButton>
                    </DialogTitle>

                    <DialogContent sx={{ p: 4 }}>
                        <Grid container spacing={4}>
                            {/* Product image and info */}
                            <Grid item xs={12} md={4} sx={{
                                opacity: showDialog ? 1 : 0,
                                transform: showDialog ? 'translateY(0)' : 'translateY(20px)',
                                transition: 'opacity 0.3s ease, transform 0.3s ease',
                                transitionDelay: '0.1s'
                            }}>
                                <Paper elevation={2} sx={{ p: 3, mb: 3, borderRadius: 2 }}>
                                    <Box sx={{ mb: 2, borderRadius: 2, overflow: 'hidden' }}>
                                        {product ? (
                                            <div style={{ height: '200px' }}>
                                                <ProductImage
                                                    imageUrl={product.primaryImageUrl || (product.images && product.images.length > 0 ? product.images[0].uri : null)}
                                                    productName={product.name}
                                                />
                                            </div>
                                        ) : (
                                            <img
                                                src={'https://via.placeholder.com/300x300?text=Product+Image'}
                                                alt="Product"
                                                style={{ width: '100%', height: 'auto', borderRadius: 8 }}
                                            />
                                        )}
                                    </Box>
                                    <Typography variant="h6" gutterBottom>
                                        {product?.name || 'Product Name'}
                                    </Typography>
                                    <Typography variant="body2" color="text.secondary" gutterBottom>
                                        {product?.brands?.[0] || 'Brand'}
                                    </Typography>
                                </Paper>
                            </Grid>

                            {/* Campaign configuration */}
                            <Grid item xs={12} md={8}>
                                <Box sx={{
                                    opacity: showDialog ? 1 : 0,
                                    transform: showDialog ? 'translateY(0)' : 'translateY(20px)',
                                    transition: 'opacity 0.3s ease, transform 0.3s ease',
                                    transitionDelay: '0.2s'
                                }}>
                                    <Typography variant="h6" gutterBottom>
                                        Campaign Configuration
                                    </Typography>

                                    <Grid container spacing={2}>
                                        {/* Content Type Dropdown */}
                                        <Grid item xs={12} md={6}>
                                            <FormControl fullWidth>
                                                <InputLabel id="content-type-label">Content Type</InputLabel>
                                                <Select
                                                    labelId="content-type-label"
                                                    id="content-type"
                                                    value={contentType}
                                                    label="Content Type"
                                                    onChange={handleContentTypeChange}
                                                    disabled={isGenerating}
                                                >
                                                    {Object.entries(contentTypeOptions).map(([key, description]) => (
                                                        <MenuItem key={key} value={key}>
                                                            <Box>
                                                                <Typography variant="body1">{key.replace('_', ' ').replace(/\b\w/g, l => l.toUpperCase())}</Typography>
                                                                <Typography variant="caption" color="text.secondary">{description}</Typography>
                                                            </Box>
                                                        </MenuItem>
                                                    ))}
                                                </Select>
                                            </FormControl>
                                        </Grid>

                                        {/* Tone Dropdown */}
                                        <Grid item xs={12} md={6}>
                                            <FormControl fullWidth>
                                                <InputLabel id="tone-label">Tone</InputLabel>
                                                <Select
                                                    labelId="tone-label"
                                                    id="tone"
                                                    value={tone}
                                                    label="Tone"
                                                    onChange={handleToneChange}
                                                    disabled={isGenerating}
                                                >
                                                    {Object.entries(toneOptions).map(([key, description]) => (
                                                        <MenuItem key={key} value={key}>
                                                            <Box>
                                                                <Typography variant="body1">{key.charAt(0).toUpperCase() + key.slice(1)}</Typography>
                                                                <Typography variant="caption" color="text.secondary">{description}</Typography>
                                                            </Box>
                                                        </MenuItem>
                                                    ))}
                                                </Select>
                                            </FormControl>
                                        </Grid>

                                        {/* Generate Button */}
                                        <Grid item xs={12} sx={{ display: 'flex', justifyContent: 'flex-end', mt: 2 }}>
                                            <Button
                                                variant="contained"
                                                color="primary"
                                                onClick={handleGenerateContent}
                                                disabled={isGenerating}
                                                startIcon={isGenerating ? <CircularProgress size={20} color="inherit" /> : null}
                                                sx={{
                                                    backgroundColor: '#4285f4',
                                                    '&:hover': {
                                                        backgroundColor: 'rgba(66, 133, 244, 0.9)',
                                                    },
                                                    transform: 'translateZ(0)',
                                                    transition: 'transform 0.2s ease-in-out',
                                                    '&:active': {
                                                        transform: 'scale(0.95)'
                                                    }
                                                }}
                                            >
                                                {isGenerating ? 'Generating...' : 'Generate Campaign'}
                                            </Button>
                                        </Grid>
                                    </Grid>
                                </Box>

                                {/* Generated content */}
                                {(isGenerating || generatedContent) && (
                                    <Box sx={{ mt: 4 }}>
                                        <Typography variant="h6" gutterBottom>
                                            Generated Content
                                        </Typography>

                                        {isGenerating ? (
                                            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', py: 4 }}>
                                                <CircularProgress />
                                                <Typography variant="body2" sx={{ ml: 2 }}>
                                                    Generating campaign with Gemini AI...
                                                </Typography>
                                            </Box>
                                        ) : generatedContent ? (
                                            <Fade in={showContent} timeout={1000}>
                                                <Paper
                                                    elevation={3}
                                                    sx={{
                                                        p: 3,
                                                        maxHeight: '300px',
                                                        overflow: 'auto',
                                                        bgcolor: '#f9f9f9',
                                                        borderLeft: '4px solid #4285f4'
                                                    }}
                                                >
                                                    <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>
                                                        {generatedContent}
                                                    </Typography>
                                                </Paper>
                                            </Fade>
                                        ) : null}

                                        {error && (
                                            <Paper
                                                elevation={3}
                                                sx={{
                                                    p: 2,
                                                    mt: 2,
                                                    bgcolor: '#fdeded',
                                                    borderLeft: '4px solid #d32f2f'
                                                }}
                                            >
                                                <Typography color="error">{error}</Typography>
                                            </Paper>
                                        )}
                                    </Box>
                                )}
                            </Grid>
                        </Grid>
                    </DialogContent>

                    {generatedContent && (
                        <DialogActions sx={{ p: 3, justifyContent: 'space-between' }}>
                            <Button
                                variant="outlined"
                                startIcon={<PersonAddIcon />}
                                onClick={handleSelectUserBase}
                                disabled={isSelectingUsers || isSending}
                                sx={{
                                    borderColor: '#D32F2F',
                                    color: '#D32F2F',
                                    '&:hover': {
                                        borderColor: '#D32F2F',
                                        backgroundColor: 'rgba(211, 47, 47, 0.04)'
                                    }
                                }}
                            >
                                {isSelectingUsers ? (
                                    <>
                                        <CircularProgress size={20} color="inherit" sx={{ mr: 1 }} />
                                        Selecting...
                                    </>
                                ) : 'Select User Base'}
                            </Button>

                            <Button
                                variant="contained"
                                endIcon={<SendIcon />}
                                onClick={handleSendCampaign}
                                disabled={isSending || isSelectingUsers}
                                sx={{
                                    backgroundColor: '#1E8E3E',
                                    '&:hover': {
                                        backgroundColor: 'rgba(30, 142, 62, 0.9)'
                                    }
                                }}
                            >
                                {isSending ? (
                                    <>
                                        <CircularProgress size={20} color="inherit" sx={{ mr: 1 }} />
                                        Sending...
                                    </>
                                ) : 'Send Campaign'}
                            </Button>
                        </DialogActions>
                    )}
                </Box>
            </Grow>
        </Dialog>
    );
};

export default MarketingCampaignDialog;
