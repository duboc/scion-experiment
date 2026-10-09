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
    Box,
    Typography,
    Paper,
    Divider,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableRow,
    TableHead
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import RemoveIcon from '@mui/icons-material/Remove';

// ContentDiff component displays differences between original and enhanced content
const ContentDiff = ({ contentType, originalContent, enhancedContent }) => {
    // Rendering logic based on content type
    const renderDiff = () => {
        switch (contentType) {
            case 'image':
                return renderImageDiff();
            case 'description':
                return renderTextDiff();
            case 'specifications':
                return renderSpecsDiff();
            default:
                return <Typography>No diff available for this content type</Typography>;
        }
    };

    // Render image comparison
    const renderImageDiff = () => {
        return (
            <Box sx={{ display: 'flex', flexDirection: { xs: 'column', md: 'row' }, gap: 2 }}>
                <Paper
                    elevation={2}
                    sx={{
                        flex: 1,
                        p: 2,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center'
                    }}
                >
                    <Typography variant="subtitle1" gutterBottom>Original Image</Typography>
                    <Box
                        component="img"
                        src={originalContent}
                        alt="Original product"
                        sx={{ width: '100%', maxHeight: 300, objectFit: 'contain' }}
                    />
                </Paper>

                <Paper
                    elevation={2}
                    sx={{
                        flex: 1,
                        p: 2,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        bgcolor: 'rgba(138, 43, 226, 0.05)'
                    }}
                >
                    <Typography variant="subtitle1" gutterBottom color="secondary">
                        Enhanced Image (AI Generated)
                    </Typography>
                    <Box
                        component="img"
                        src={enhancedContent}
                        alt="AI enhanced product"
                        sx={{ width: '100%', maxHeight: 300, objectFit: 'contain' }}
                    />
                </Paper>
            </Box>
        );
    };

    // Render text content diff
    const renderTextDiff = () => {
        // In a real implementation, you would use a proper diff algorithm
        // For this mock, we'll simulate by highlighting the entire enhanced content

        return (
            <Box>
                <Typography variant="subtitle1" gutterBottom>Content Differences:</Typography>

                <Paper elevation={1} sx={{ p: 2, mb: 2, bgcolor: '#FFEFEF' }}>
                    <Typography variant="body2" sx={{
                        fontFamily: 'monospace',
                        textDecoration: 'line-through',
                        color: '#D32F2F'
                    }}>
                        {originalContent}
                    </Typography>
                </Paper>

                <Paper elevation={1} sx={{ p: 2, bgcolor: '#E6F4EA' }}>
                    <Typography variant="body2" sx={{
                        fontFamily: 'monospace',
                        color: '#1E8E3E'
                    }}>
                        {enhancedContent}
                    </Typography>
                </Paper>
            </Box>
        );
    };

    // Render specifications diff as a table
    const renderSpecsDiff = () => {
        // Assume specs are in format: [{ name, value, isNew }]
        return (
            <Box>
                <Typography variant="subtitle1" gutterBottom>Specification Changes:</Typography>

                <TableContainer component={Paper}>
                    <Table size="small">
                        <TableHead>
                            <TableRow>
                                <TableCell sx={{ fontWeight: 'bold' }}>Specification</TableCell>
                                <TableCell sx={{ fontWeight: 'bold' }}>Value</TableCell>
                                <TableCell sx={{ fontWeight: 'bold' }}>Status</TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {enhancedContent.map((spec, index) => {
                                // Find if this spec already exists in original content
                                const originalSpec = originalContent.find(
                                    os => os.name.toLowerCase() === spec.name.toLowerCase()
                                );

                                // Determine if it's new, changed, or unchanged
                                const isExistingSpec = !!originalSpec;
                                const isChanged = isExistingSpec && originalSpec.value !== spec.value;
                                const isNew = !isExistingSpec;

                                // Only mark as new if it doesn't exist or the value changed
                                const displayStatus = isNew ? "Added" : (isChanged ? "Changed" : "Unchanged");
                                const statusColor = isNew ? '#1E8E3E' : (isChanged ? '#1976D2' : '#757575');
                                const bgColor = isNew ? 'rgba(30, 142, 62, 0.1)' :
                                    (isChanged ? 'rgba(25, 118, 210, 0.1)' : 'transparent');

                                return (
                                    <TableRow
                                        key={index}
                                        sx={{ bgcolor: bgColor }}
                                    >
                                        <TableCell>{spec.name}</TableCell>
                                        <TableCell>{spec.value}</TableCell>
                                        <TableCell>
                                            <Box sx={{ display: 'flex', alignItems: 'center', color: statusColor }}>
                                                {isNew && <AddIcon fontSize="small" sx={{ mr: 0.5 }} />}
                                                <Typography variant="caption">{displayStatus}</Typography>
                                            </Box>
                                        </TableCell>
                                    </TableRow>
                                );
                            })}
                        </TableBody>
                    </Table>
                </TableContainer>
            </Box>
        );
    };

    return (
        <Box sx={{ mt: 3, mb: 3 }}>
            <Paper
                elevation={3}
                sx={{
                    p: 3,
                    border: '1px solid rgba(138, 43, 226, 0.3)',
                    borderRadius: 2
                }}
            >
                <Typography
                    variant="h6"
                    gutterBottom
                    sx={{
                        color: 'secondary.main',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        mb: 2
                    }}
                >
                    AI Enhanced Content Preview
                </Typography>

                <Divider sx={{ mb: 3 }} />

                {renderDiff()}
            </Paper>
        </Box>
    );
};

export default ContentDiff;
