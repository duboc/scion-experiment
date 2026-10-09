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

import React, { useState } from 'react';
import { Box, Typography, IconButton, Tooltip, CircularProgress } from '@mui/material';
import UpdateIcon from '@mui/icons-material/Update';
import CancelIcon from '@mui/icons-material/Cancel';
import AutoFixHighIcon from '@mui/icons-material/AutoFixHigh';
import geminiIcon from '../assets/gemini.png';

// Custom Gemini icon
export const GeminiIcon = (props) => {
    return (
        <img
            src={geminiIcon}
            alt="Gemini AI"
            style={{
                width: '20px',
                height: '20px',
                display: 'block',
                margin: '0 auto'
            }}
        />
    );
};

// EnhanceableContent is a wrapper component that adds AI enhancement capabilities
// to any content it wraps
const EnhanceableContent = ({
    children,
    contentType,
    onEnhance,
    onUpdate,
    enhancementState,
}) => {
    const [isHovered, setIsHovered] = useState(false);

    // Determine what button/state to show
    const showEnhanceButton = enhancementState === 'idle' && isHovered;
    const showLoading = enhancementState === 'enhancing';
    const showUpdateButton = enhancementState === 'showingDiff';

    const handleEnhanceClick = () => {
        onEnhance(contentType);
    };

    const handleUpdateClick = () => {
        onUpdate(contentType);
    };

    return (
        <Box
            position="relative"
            onMouseEnter={() => setIsHovered(true)}
            onMouseLeave={() => setIsHovered(false)}
            sx={{
                transition: 'all 0.3s',
                '&:hover': {
                    boxShadow: enhancementState === 'idle' ? '0 0 0 2px rgba(66, 133, 244, 0.3)' : 'none',
                },
            }}
        >
            {/* The actual content */}
            {children}

            {/* Enhancement button - shown on hover */}
            {showEnhanceButton && (
                <Tooltip title="Enhance with Gemini AI">
                    <IconButton
                        onClick={handleEnhanceClick}
                        sx={{
                            position: 'absolute',
                            top: 10,
                            right: 10,
                            backgroundColor: 'rgba(66, 133, 244, 0.8)', // Blue theme
                            color: 'white',
                            '&:hover': {
                                backgroundColor: 'rgba(66, 133, 244, 1)',
                            },
                            zIndex: 10,
                        }}
                    >
                        <GeminiIcon />
                    </IconButton>
                </Tooltip>
            )}

            {/* Enhancement completed badge */}
            {enhancementState === 'updated' && (
                <Tooltip title="Enhanced with AI">
                    <Box
                        sx={{
                            position: 'absolute',
                            top: 10,
                            right: 10,
                            backgroundColor: 'rgba(66, 133, 244, 0.8)',
                            color: 'white',
                            borderRadius: '50%',
                            width: 32,
                            height: 32,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            zIndex: 10,
                        }}
                    >
                        <AutoFixHighIcon fontSize="small" />
                    </Box>
                </Tooltip>
            )}

            {/* Loading indicator */}
            {showLoading && (
                <Box
                    sx={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        bottom: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        backgroundColor: 'rgba(255, 255, 255, 0.9)',
                        zIndex: 10,
                        backdropFilter: 'blur(2px)',
                        border: '1px solid rgba(66, 133, 244, 0.2)',
                        borderRadius: '4px',
                    }}
                >
                    <Box sx={{ position: 'relative', display: 'inline-flex' }}>
                        <CircularProgress color="secondary" size={60} thickness={4} />
                        <Box
                            sx={{
                                top: 0,
                                left: 0,
                                bottom: 0,
                                right: 0,
                                position: 'absolute',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                            }}
                        >
                            <GeminiIcon />
                        </Box>
                    </Box>
                    <Typography variant="body1" sx={{ mt: 3, mb: 1, color: 'text.primary', fontWeight: 'medium' }}>
                        Generating AI-enhanced content
                    </Typography>
                    <Typography variant="body2" sx={{ color: 'text.secondary', maxWidth: '80%', textAlign: 'center' }}>
                        This may take a few moments while Gemini analyzes and enhances your {contentType}...
                    </Typography>
                </Box>
            )}

            {/* Update catalog button and Cancel button */}
            {showUpdateButton && (
                <Box sx={{ position: 'absolute', top: 10, right: 10, display: 'flex', gap: 1, zIndex: 10 }}>
                    <Tooltip title="Update catalog with AI-enhanced content">
                        <IconButton
                            onClick={handleUpdateClick}
                            sx={{
                                backgroundColor: 'rgba(30, 142, 62, 0.9)',  // Green color from ContentDiff (#1E8E3E)
                                color: 'white',
                                '&:hover': {
                                    backgroundColor: '#1E8E3E',  // Solid on hover
                                },
                            }}
                        >
                            <UpdateIcon />
                        </IconButton>
                    </Tooltip>

                    <Tooltip title="Cancel and return to original content">
                        <IconButton
                            onClick={() => onEnhance('cancel')}
                            sx={{
                                backgroundColor: 'rgba(211, 47, 47, 0.9)',  // Red color from ContentDiff (#D32F2F)
                                color: 'white',
                                '&:hover': {
                                    backgroundColor: '#D32F2F',  // Solid on hover
                                },
                            }}
                        >
                            <CancelIcon />
                        </IconButton>
                    </Tooltip>
                </Box>
            )}
        </Box>
    );
};

export default EnhanceableContent;
